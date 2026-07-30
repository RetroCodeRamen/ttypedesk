// Package lanchat is a fully decentralized, LAN-only peer-to-peer
// messenger — no server, no homeserver, no internet dependency. Peers
// announce themselves via a UDP broadcast beacon, identify themselves
// with a locally-generated, persisted Ed25519 keypair (plus a
// user-chosen display name), and exchange messages over direct TCP
// connections. Anyone can create a room; a room's message history is
// gossiped between whichever LAN peers have joined it, so a newly-joined
// peer converges on the same history everyone else already has. Each
// peer keeps only the most recent maxRoomMessages messages per room,
// persisted to disk. Direct messages are just a room with exactly two
// members, whose ID both participants derive independently — see
// DMRoom.
//
// Messages are signed (so you can verify who really sent something and
// detect tampering) but not encrypted — content travels in the clear on
// the LAN. That's a deliberate scope decision: a trusted home/office
// network doesn't need it, and hand-rolled end-to-end encryption is a
// common source of real vulnerabilities when done as an afterthought.
//
// Service is a single, process-lifetime background service (like
// internal/notify.Service) — instantiated once in internal/server, not
// per-window, so discovery and message receipt keep running even when no
// Chat window is open. apps/chat wraps it for the UI.
package lanchat

import (
	"crypto/ed25519"
	"net"
	"sync"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/notify"
)

// maxRoomMessages is how many messages each peer keeps per room, both in
// memory and on disk — the newest maxRoomMessages, oldest dropped first.
const maxRoomMessages = 500

// PeerID is a peer's Ed25519 public key, hex-encoded — both a stable
// identity and directly verifiable (no separate registration or trust
// step; the key *is* the identity).
type PeerID string

// RoomID identifies a room — either a normal room (an opaque hash
// generated at creation, see CreateRoom) or a DM room (a hash
// deterministic from the two participants' PeerIDs, see DMRoom).
type RoomID string

// Message is one chat message, signed by its sender.
type Message struct {
	ID         string    `json:"id"` // content-derived, used for dedup — see wire.go's messageID
	Sender     PeerID    `json:"sender"`
	SenderName string    `json:"sender_name"` // sender's display name *at send time* — not re-resolved later
	Body       string    `json:"body"`
	Time       time.Time `json:"time"`
	Sig        []byte    `json:"sig"`
}

// RoomSummary is a room's metadata, without its message history — what
// Rooms() returns for a room list UI.
type RoomSummary struct {
	ID        RoomID
	Name      string
	IsDM      bool
	DMPeer    PeerID // set only when IsDM
	Joined    bool   // true once the local user has actually joined (CreateRoom/JoinRoom/DMRoom)
	CreatedBy PeerID
	CreatedAt time.Time
}

// PeerSummary is a currently-known LAN peer — online if seen recently
// via its discovery beacon (see discovery.go's peerTimeout).
type PeerSummary struct {
	ID     PeerID
	Name   string
	Online bool
}

// Event is delivered to Subscribe callbacks — see notify.Service.Subscribe
// for the identical pattern this mirrors.
type Event struct {
	Kind EventKind
	Room RoomID  // set for EventMessage/EventRoomAnnounced
	Peer PeerID  // set for EventPeerOnline/EventPeerOffline
	Msg  Message // set for EventMessage
}

type EventKind int

const (
	EventMessage       EventKind = iota // a new message landed in Room (sent locally or received)
	EventRoomAnnounced                  // a new room became known (not necessarily joined)
	EventPeerOnline                     // Peer's beacon was seen for the first time (or after being offline)
	EventPeerOffline                    // Peer hasn't been seen within peerTimeout
)

// Service is the shared background LAN chat engine — see the package
// doc. Safe for concurrent use.
type Service struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	self PeerID

	dataDir string

	mu          sync.Mutex
	displayName string
	peers       map[PeerID]*peerState
	conns       map[PeerID]*peerConn
	rooms       map[RoomID]*roomState
	listeners   []func(Event)

	// rawConns tracks every live net.Conn from the moment it's dialed or
	// accepted — before the hello handshake completes and it earns a
	// PeerID-keyed entry in conns. Close needs this: a connection stuck
	// mid-handshake (or one whose peer never replies) isn't in conns
	// yet, but its handleConn goroutine is still blocked in a Read that
	// only Close()-ing its net.Conn can unblock — closing only what's in
	// conns would leak that goroutine (and hang Close's wg.Wait())
	// forever.
	rawConns map[net.Conn]struct{}

	udpConn  udpConn
	tcpLn    tcpListener
	tcpPort  int
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	notify *notify.Service // may be nil (tests, or not yet wired) — see applyIncomingMessage
}

// peerState is what discovery.go tracks per known peer.
type peerState struct {
	name     string
	addr     string // host:tcpPort, from the most recent beacon
	lastSeen time.Time
	online   bool
}

// roomState is a room's full local state — RoomSummary plus its message
// history and gossip bookkeeping.
type roomState struct {
	RoomSummary
	messages []Message       // newest last, capped at maxRoomMessages
	seen     map[string]bool // message IDs already applied, for dedup
	subs     map[PeerID]bool // peers who've sent us room_join for this room (fan-out targets)

	// known is false for a roomState that exists purely as room_join
	// subscriber bookkeeping (see applyRoomJoin) for a room we've never
	// actually seen created/announced or joined ourselves — most
	// commonly a DM room between two *other* peers, whose room_join is
	// still broadcast to us so message fan-out logic works uniformly,
	// but which shouldn't appear as a visible/known room from our side.
	// Rooms() filters these out; true for anything from CreateRoom,
	// DMRoom, applyRoomAnnounce, or a loaded/ingested message.
	known bool
}

// Subscribe registers fn to be called for every future Event. No
// unsubscribe mechanism — mirrors internal/notify.Service.Subscribe
// exactly, same reasoning: nothing in this codebase has ever needed one.
func (s *Service) Subscribe(fn func(Event)) {
	s.mu.Lock()
	s.listeners = append(s.listeners, fn)
	s.mu.Unlock()
}

// emit copies the listener slice under the lock and calls them unlocked
// — so a listener that itself calls back into the Service can't deadlock
// (identical reasoning/shape to internal/notify.Service.Post).
func (s *Service) emit(ev Event) {
	s.mu.Lock()
	listeners := append([]func(Event){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(ev)
	}
}

// Self returns this Service's own identity — its PeerID and current
// display name.
func (s *Service) Self() (PeerID, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.self, s.displayName
}
