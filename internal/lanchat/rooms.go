package lanchat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// CreateRoom creates and joins a new room, then gossips its existence
// (room_announce) and membership (room_join) to every currently
// connected peer. The room's ID has no central allocation — it's a hash
// of the creator's PeerID, the name, and a nanosecond timestamp, so two
// people creating same-named rooms independently and simultaneously get
// different, non-colliding IDs (see the plan's note on rooms having no
// global uniqueness).
func (s *Service) CreateRoom(name string) RoomID {
	self := s.selfID()
	now := time.Now()
	id := roomID(self, name, now.UnixNano())

	s.mu.Lock()
	s.rooms[id] = &roomState{
		RoomSummary: RoomSummary{
			ID:        id,
			Name:      name,
			Joined:    true,
			CreatedBy: self,
			CreatedAt: now,
		},
		seen:  map[string]bool{},
		subs:  map[PeerID]bool{},
		known: true,
	}
	s.mu.Unlock()

	s.persistRoom(id)
	s.broadcastToConns(wireRoomAnnounce, wireRoomAnnounceMsg{
		RoomID: id, Name: name, CreatedBy: self, CreatedAt: now.UnixNano(),
	}, "")
	s.broadcastToConns(wireRoomJoin, wireRoomJoinMsg{RoomID: id, PeerID: self}, "")
	return id
}

// JoinRoom joins a room this instance already knows about (typically via
// a prior room_announce) but hasn't joined yet — announces membership so
// peers start sending us messages and, if any peer is already a member,
// its history.
func (s *Service) JoinRoom(id RoomID) {
	s.mu.Lock()
	r, ok := s.rooms[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	r.Joined = true
	s.mu.Unlock()

	s.persistRoom(id)
	s.broadcastToConns(wireRoomJoin, wireRoomJoinMsg{RoomID: id, PeerID: s.selfID()}, "")
}

// DMRoom returns the room ID for a direct-message conversation with
// peer, joining it locally (idempotent — safe to call every time a DM
// is opened) if this is the first time. Both participants derive the
// exact same ID independently — sorted(self, peer) hashed — so there's
// no handshake needed to "start" a DM, and it reuses every room
// mechanism (gossip, 500-message cap, persistence) unchanged. It's never
// room_announce'd (DMs aren't discoverable), but room_join *is*
// broadcast to every connection like any other room — see the package
// doc and handleEnvelope's room_join case for why that doesn't leak a
// DM's existence or content to anyone but the real other participant.
func (s *Service) DMRoom(peer PeerID) RoomID {
	self := s.selfID()
	id := dmRoomID(self, peer)

	s.mu.Lock()
	if r, ok := s.rooms[id]; !ok {
		s.rooms[id] = &roomState{
			RoomSummary: RoomSummary{
				ID:     id,
				IsDM:   true,
				DMPeer: peer,
				Joined: true,
			},
			seen:  map[string]bool{},
			subs:  map[PeerID]bool{},
			known: true,
		}
	} else {
		r.Joined = true
		r.known = true
	}
	s.mu.Unlock()

	s.persistRoom(id)
	s.broadcastToConns(wireRoomJoin, wireRoomJoinMsg{RoomID: id, PeerID: self}, "")
	return id
}

// SendMessage signs body as a new message in room, stores it locally,
// and floods it to every peer subscribed to that room (peers who've
// sent us a room_join for it).
func (s *Service) SendMessage(room RoomID, body string) error {
	s.mu.Lock()
	r, ok := s.rooms[room]
	if !ok || !r.Joined {
		s.mu.Unlock()
		return fmt.Errorf("lanchat: not a member of room %q", room)
	}
	priv, self, name := s.priv, s.self, s.displayName
	s.mu.Unlock()

	m := signMessage(priv, self, name, room, body, time.Now())

	s.mu.Lock()
	r, ok = s.rooms[room]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("lanchat: room %q no longer exists", room)
	}
	r.messages = appendCapped(r.messages, m)
	r.seen[m.ID] = true
	targets := subsSlice(r.subs)
	s.mu.Unlock()

	s.persistRoom(room)
	s.emit(Event{Kind: EventMessage, Room: room, Msg: m})
	s.sendMessageTo(targets, room, m)
	return nil
}

// applyRoomAnnounce records a newly-learned room (without joining it)
// and re-gossips it onward to other connected peers the first time it's
// seen, so announcements propagate beyond directly-connected peers.
func (s *Service) applyRoomAnnounce(msg wireRoomAnnounceMsg) {
	s.mu.Lock()
	r, exists := s.rooms[msg.RoomID]
	alreadyAnnounced := exists && r.known
	if exists && !r.known {
		// Promote a bare room_join subscriber stub (see roomState.known's
		// doc comment) into a real, visible room now that we've actually
		// learned it exists.
		r.Name = msg.Name
		r.CreatedBy = msg.CreatedBy
		r.CreatedAt = time.Unix(0, msg.CreatedAt)
		r.known = true
	} else if !exists {
		s.rooms[msg.RoomID] = &roomState{
			RoomSummary: RoomSummary{
				ID:        msg.RoomID,
				Name:      msg.Name,
				CreatedBy: msg.CreatedBy,
				CreatedAt: time.Unix(0, msg.CreatedAt),
			},
			seen:  map[string]bool{},
			subs:  map[PeerID]bool{},
			known: true,
		}
	}
	s.mu.Unlock()

	if alreadyAnnounced {
		return
	}
	s.emit(Event{Kind: EventRoomAnnounced, Room: msg.RoomID})
	s.broadcastToConns(wireRoomAnnounce, msg, "")
}

// applyRoomJoin records that joiner is a member of id: adds it as a
// fan-out target for future messages, replies with our own history for
// that room — sent directly to joiner, not to from — if we're a member
// too, and — the first time we hear about this joiner for this room —
// re-gossips the join onward so multi-hop mesh convergence works, not
// just direct connections.
//
// joiner is carried explicitly in the wire message rather than inferred
// from from (the connection this copy happened to arrive over):
// room_join is relayed verbatim by peers who aren't themselves members
// (see the re-gossip call below), and without an explicit origin, a
// relay's own connection would be indistinguishable from a real member —
// letting any relay on the mesh silently add itself as a subscriber to
// a room (including a private DM) it was never actually joined to. See
// wireRoomJoinMsg's doc comment.
//
// For the same reason, the history_sync reply must go out over our own
// direct connection to joiner, never over from: from is just whichever
// physical link this particular copy happened to travel over, which for
// a relayed join is the relay's link, not the joiner's — replying to
// from would hand the room's history to that relay instead of the
// actual joiner. If we have no direct connection to joiner (pure
// multi-hop, never having discovered/dialed them ourselves), we simply
// can't reply — acceptable under this package's gossip model, which
// doesn't attempt store-and-forward relaying of history through
// intermediate peers.
func (s *Service) applyRoomJoin(from *peerConn, id RoomID, joiner PeerID) {
	if joiner == "" || joiner == s.selfID() {
		return
	}
	s.mu.Lock()
	r, ok := s.rooms[id]
	if !ok {
		r = &roomState{
			RoomSummary: RoomSummary{ID: id},
			seen:        map[string]bool{},
			subs:        map[PeerID]bool{},
		}
		s.rooms[id] = r
	}
	alreadySub := r.subs[joiner]
	r.subs[joiner] = true
	weAreMember := r.Joined
	var history []Message
	if weAreMember {
		history = append(history, r.messages...)
	}
	directConn := s.conns[joiner]
	s.mu.Unlock()

	if weAreMember && directConn != nil {
		directConn.send(wireHistorySync, wireHistorySyncMsg{RoomID: id, Messages: history})
	}
	if !alreadySub {
		s.broadcastToConns(wireRoomJoin, wireRoomJoinMsg{RoomID: id, PeerID: joiner}, from.id)
	}
}

// applyHistorySync merges a peer's reported history for a room into our
// own, deduping by content-hash ID exactly like applyIncomingMessage —
// history_sync is just a batch of the same kind of message.
func (s *Service) applyHistorySync(msg wireHistorySyncMsg) {
	changed := false
	for _, m := range msg.Messages {
		if s.ingestMessage(msg.RoomID, m) {
			changed = true
		}
	}
	if changed {
		s.persistRoom(msg.RoomID)
	}
}

// applyIncomingMessage ingests a single freshly-gossiped message and, if
// it was new (not a duplicate we'd already applied), re-floods it to our
// own subscribers other than whoever just sent it to us.
func (s *Service) applyIncomingMessage(room RoomID, m Message, from PeerID) {
	if !s.ingestMessage(room, m) {
		return
	}
	s.persistRoom(room)
	s.emit(Event{Kind: EventMessage, Room: room, Msg: m})
	if s.notify != nil {
		s.notify.Post(m.SenderName, m.Body, "💬", "lanchat")
	}

	s.mu.Lock()
	var targets []PeerID
	if r, ok := s.rooms[room]; ok {
		targets = subsSlice(r.subs)
	}
	s.mu.Unlock()
	s.sendMessageTo(removePeer(targets, from), room, m)
}

// ingestMessage verifies, dedups, and stores m into room's local
// history, trimmed to the newest maxRoomMessages. Returns true if m was
// newly added (false for a duplicate or a signature that didn't
// verify).
func (s *Service) ingestMessage(room RoomID, m Message) bool {
	if err := verifyMessage(room, m); err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rooms[room]
	if !ok {
		r = &roomState{
			RoomSummary: RoomSummary{ID: room},
			seen:        map[string]bool{},
			subs:        map[PeerID]bool{},
			known:       true,
		}
		s.rooms[room] = r
	} else if !r.known {
		r.known = true
	}
	if r.seen[m.ID] {
		return false
	}
	r.seen[m.ID] = true
	r.messages = appendCapped(r.messages, m)
	return true
}

func (s *Service) sendMessageTo(targets []PeerID, room RoomID, m Message) {
	if len(targets) == 0 {
		return
	}
	s.mu.Lock()
	var conns []*peerConn
	for _, id := range targets {
		if pc, ok := s.conns[id]; ok {
			conns = append(conns, pc)
		}
	}
	s.mu.Unlock()
	for _, pc := range conns {
		pc.send(wireMessage, wireMessageMsg{RoomID: room, Msg: m})
	}
}

// appendCapped appends m in time order and trims the result to the
// newest maxRoomMessages entries.
func appendCapped(msgs []Message, m Message) []Message {
	msgs = append(msgs, m)
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Time.Before(msgs[j].Time) })
	if len(msgs) > maxRoomMessages {
		msgs = msgs[len(msgs)-maxRoomMessages:]
	}
	return msgs
}

func subsSlice(subs map[PeerID]bool) []PeerID {
	out := make([]PeerID, 0, len(subs))
	for id := range subs {
		out = append(out, id)
	}
	return out
}

func removePeer(ids []PeerID, remove PeerID) []PeerID {
	out := ids[:0]
	for _, id := range ids {
		if id != remove {
			out = append(out, id)
		}
	}
	return out
}

func roomID(creator PeerID, name string, tsNano int64) RoomID {
	sum := sha256.Sum256([]byte(string(creator) + "\x00" + name + "\x00" + fmt.Sprintf("%d", tsNano)))
	return RoomID(hex.EncodeToString(sum[:]))
}

// dmRoomID is order-independent so both participants derive the same ID
// regardless of who's "self" and who's "peer".
func dmRoomID(a, b PeerID) RoomID {
	if b < a {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte("dm\x00" + string(a) + "\x00" + string(b)))
	return RoomID(hex.EncodeToString(sum[:]))
}
