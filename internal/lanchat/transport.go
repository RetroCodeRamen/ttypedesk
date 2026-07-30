package lanchat

import (
	"bufio"
	"net"
	"sync"
	"time"
)

// tcpListener is the subset of *net.TCPListener transport.go needs —
// narrowed to an interface for the same reason as udpConn.
type tcpListener interface {
	Accept() (net.Conn, error)
	Close() error
	Addr() net.Addr
}

// peerConn is one live TCP connection to a peer, plus the bookkeeping
// needed to write to it safely from multiple goroutines (message gossip
// can originate from SendMessage, from re-gossiping something just
// received from a different peer, or from the initial post-hello
// room_join replay — all potentially concurrent).
type peerConn struct {
	id        PeerID
	nc        net.Conn
	mu        sync.Mutex
	w         *bufio.Writer
	canonical bool // see handleConn's dedup logic
}

func (pc *peerConn) send(typ wireType, payload any) error {
	data, err := encodeWire(typ, payload)
	if err != nil {
		return err
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if _, err := pc.w.Write(data); err != nil {
		return err
	}
	if err := pc.w.WriteByte('\n'); err != nil {
		return err
	}
	return pc.w.Flush()
}

// listenTCP opens the transport listener on an OS-assigned port — see
// the package doc's note on why this is dynamic while discoveryPort is
// fixed.
func listenTCP() (*net.TCPListener, error) {
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: 0})
	if err != nil {
		return nil, err
	}
	return ln, nil
}

// acceptLoop accepts inbound connections until stopCh closes/the
// listener is closed.
func (s *Service) acceptLoop() {
	defer s.wg.Done()
	for {
		nc, err := s.tcpLn.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(nc, false)
	}
}

// connectIfNeeded dials peer if the connection-dedup rule says we should
// be the dialing side and we don't already have a live connection to
// them: the lexicographically smaller PeerID always dials, the larger
// always waits to accept — deterministic, no extra coordination or
// races between both sides trying to dial simultaneously.
func (s *Service) connectIfNeeded(peer PeerID) {
	if s.selfID() >= peer {
		return
	}
	s.mu.Lock()
	_, connected := s.conns[peer]
	ps, known := s.peers[peer]
	addr := ""
	if known {
		addr = ps.addr
	}
	s.mu.Unlock()
	if connected || !known || addr == "" {
		return
	}

	s.wg.Add(1)
	go func() {
		nc, err := net.DialTimeout("tcp4", addr, 5*time.Second)
		if err != nil {
			s.wg.Done()
			return
		}
		// Run handleConn in this same goroutine (not a further nested
		// `go`) so the single Add(1) above covers its whole lifetime —
		// no second Add() call that could race against a concurrent
		// Close()'s Wait() ever seeing the counter transiently at zero.
		s.handleConn(nc, true)
	}()
}

// handleConn drives one connection end-to-end: hello exchange,
// registering it in s.conns (closing any existing connection to the
// same peer — the dedup rule keeps this from racing under normal
// operation, but a stale connection surviving a peer restart is a real
// case worth handling defensively), replaying local room_join for every
// joined room, then reading and applying messages until the connection
// closes.
func (s *Service) handleConn(nc net.Conn, weDialed bool) {
	defer s.wg.Done()
	defer nc.Close()

	// Register nc in rawConns (see its doc comment) before doing any I/O
	// on it, so Close can always reach it — even if this connection never
	// makes it through the hello handshake to earn a conns entry. If
	// stopCh is already closed by the time we get the lock, Close's own
	// snapshot has already run (or never will), so there's nothing to
	// register into; just close nc and bail.
	s.mu.Lock()
	select {
	case <-s.stopCh:
		s.mu.Unlock()
		return
	default:
	}
	s.rawConns[nc] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.rawConns, nc)
		s.mu.Unlock()
	}()

	pc := &peerConn{nc: nc, w: bufio.NewWriter(nc)}
	// bufio.Scanner, not bufio.Reader.ReadBytes: ReadBytes has no maximum
	// line length and grows its buffer without bound as long as bytes
	// keep arriving without a '\n' — any connected peer (no auth gates a
	// connection on this LAN-trust-model protocol) could send an
	// unterminated stream and grow this process's memory until it's
	// killed. Buffer(_, maxWireLineBytes) makes Scan() fail once a single
	// line exceeds that cap, which — same as any other malformed input —
	// just ends this connection rather than the process.
	scanner := bufio.NewScanner(nc)
	scanner.Buffer(make([]byte, 4096), maxWireLineBytes)

	// Captured once, not re-read per use below: a consistent view of our
	// own identity for the whole handshake, and — since it's read via
	// selfID(), not the bare field — safe even if RegenerateIdentity runs
	// concurrently on another goroutine mid-handshake.
	self := s.selfID()

	if err := pc.send(wireHello, wireHelloMsg{PeerID: self, Name: s.selfName()}); err != nil {
		return
	}
	if !scanner.Scan() {
		return
	}
	env, err := decodeWire(scanner.Bytes())
	if err != nil || env.Type != wireHello {
		return
	}
	hello, err := decodeWirePayload[wireHelloMsg](env)
	if err != nil || hello.PeerID == "" || hello.PeerID == self {
		return
	}
	pc.id = hello.PeerID

	// canonical mirrors the documented dedup rule (connectIfNeeded: the
	// lower PeerID dials). It's used here for a different purpose: two
	// independent connections to the same peer can both complete their
	// handshakes (a forced DialSeed racing the peer's own passive
	// discovery-triggered dial, for instance) — and since eviction below
	// is otherwise "whichever registers last, locally," each side could
	// independently keep a *different* one of the two physical
	// connections, leaving neither shared and both eventually torn down
	// from under each other. canonical is a property of the connection
	// itself (which side dialed, compared against both IDs) that both
	// ends compute identically for the same physical connection, so
	// preferring it consistently here means both sides converge on
	// keeping the *same* survivor instead of two uncoordinated,
	// independent local choices.
	canonical := (weDialed && self < pc.id) || (!weDialed && pc.id < self)

	s.mu.Lock()
	if old, ok := s.conns[pc.id]; ok {
		if !canonical && old.canonical {
			// A non-canonical connection never displaces an existing
			// canonical one — close the redundant new connection and
			// leave the established one running.
			s.mu.Unlock()
			return
		}
		old.nc.Close()
	}
	pc.canonical = canonical
	s.conns[pc.id] = pc
	if ps, ok := s.peers[pc.id]; ok {
		ps.name = hello.Name
		ps.lastSeen = time.Now()
		if !ps.online {
			ps.online = true
			defer s.emit(Event{Kind: EventPeerOnline, Peer: pc.id})
		}
	} else {
		s.peers[pc.id] = &peerState{name: hello.Name, lastSeen: time.Now(), online: true}
		defer s.emit(Event{Kind: EventPeerOnline, Peer: pc.id})
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		wasActive := false
		if cur, ok := s.conns[pc.id]; ok && cur == pc {
			delete(s.conns, pc.id)
			wasActive = true
		}
		stillOnline := s.peers[pc.id] != nil && s.peers[pc.id].online
		s.mu.Unlock()

		// wasActive means this connection (not some already-superseded
		// duplicate) was the one peers/UI/gossip were relying on, and
		// it's now gone. If we still believe the peer is reachable
		// (its beacon hasn't timed out), immediately try to
		// reconnect — same connectIfNeeded used for the initial
		// offline→online transition, just re-triggered here so a
		// dropped connection (including two racing connections
		// mutually evicting each other down to zero — see the
		// canonical comment above) self-heals promptly instead of
		// waiting for a full offline/online cycle.
		if wasActive && stillOnline {
			select {
			case <-s.stopCh:
			default:
				s.connectIfNeeded(pc.id)
			}
		}
	}()

	s.syncOnConnect(pc)

	for scanner.Scan() {
		env, err := decodeWire(scanner.Bytes())
		if err != nil {
			continue
		}
		s.handleEnvelope(pc, env)
	}
}

// syncOnConnect brings a freshly (re)established connection up to date:
// re-announces every non-DM room we know about (so a peer that missed
// the original, one-time room_announce — because it wasn't connected
// yet, or because an earlier connection to it was replaced by a newer
// one mid-flight, see handleConn's dedup close — still converges on it),
// and replays our own room_join for every room we've personally joined,
// so the other side reciprocates with its own room_join if it's a
// member too (see rooms.go's fan-out comment for why that's safe to do
// uniformly, including for DM rooms). Run on every connection, not just
// the first one ever made to a given peer — cheap (a handful of small
// messages), and it's what makes reconnection self-healing instead of
// silently losing anything that was only ever sent once.
func (s *Service) syncOnConnect(pc *peerConn) {
	s.mu.Lock()
	var announces []wireRoomAnnounceMsg
	var joined []RoomID
	for id, r := range s.rooms {
		if !r.known {
			continue
		}
		if !r.IsDM {
			announces = append(announces, wireRoomAnnounceMsg{
				RoomID: id, Name: r.Name, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UnixNano(),
			})
		}
		if r.Joined {
			joined = append(joined, id)
		}
	}
	s.mu.Unlock()
	for _, msg := range announces {
		pc.send(wireRoomAnnounce, msg)
	}
	self := s.selfID()
	for _, id := range joined {
		pc.send(wireRoomJoin, wireRoomJoinMsg{RoomID: id, PeerID: self})
	}
}

func (s *Service) handleEnvelope(pc *peerConn, env wireEnvelope) {
	switch env.Type {
	case wireRoomAnnounce:
		msg, err := decodeWirePayload[wireRoomAnnounceMsg](env)
		if err != nil {
			return
		}
		s.applyRoomAnnounce(msg)

	case wireRoomJoin:
		msg, err := decodeWirePayload[wireRoomJoinMsg](env)
		if err != nil {
			return
		}
		s.applyRoomJoin(pc, msg.RoomID, msg.PeerID)

	case wireHistorySync:
		msg, err := decodeWirePayload[wireHistorySyncMsg](env)
		if err != nil {
			return
		}
		s.applyHistorySync(msg)

	case wireMessage:
		msg, err := decodeWirePayload[wireMessageMsg](env)
		if err != nil {
			return
		}
		s.applyIncomingMessage(msg.RoomID, msg.Msg, pc.id)
	}
}

// broadcastToConns sends payload to every currently-connected peer
// except skip (skip is the peer it was just received from, when
// re-gossiping — avoids the trivial one-hop echo; duplicate delivery
// beyond that is still possible over a longer path and is handled by
// content-hash dedup in rooms.go, not by anything here).
func (s *Service) broadcastToConns(typ wireType, payload any, skip PeerID) {
	s.mu.Lock()
	conns := make([]*peerConn, 0, len(s.conns))
	for id, pc := range s.conns {
		if id == skip {
			continue
		}
		conns = append(conns, pc)
	}
	s.mu.Unlock()
	for _, pc := range conns {
		pc.send(typ, payload)
	}
}

func (s *Service) selfName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.displayName
}
