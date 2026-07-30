package lanchat

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/RetroCodeRamen/ttypedesk/internal/notify"
)

// DefaultDataDir is ~/.config/ttypedesk/lanchat — mirrors
// notify.DefaultPath's placement under ~/.config/ttypedesk.
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "lanchat"
	}
	return filepath.Join(home, ".config", "ttypedesk", "lanchat")
}

// New creates and starts the LAN chat engine: loads (or generates) this
// instance's identity, loads any previously-persisted rooms, opens the
// UDP discovery socket and TCP transport listener, and starts its
// background goroutines. notifySvc may be nil in tests; when set, it's
// used to surface a system notification for messages received while no
// Chat window has the room open (wired in apps/chat, Phase C).
//
// dataDir is normally DefaultDataDir(); tests pass a temp directory so
// multiple instances in one process don't collide.
func New(dataDir string, notifySvc *notify.Service) (*Service, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	pub, priv, err := loadOrCreateIdentity(dataDir)
	if err != nil {
		return nil, err
	}

	udp, err := listenUDP()
	if err != nil {
		return nil, fmt.Errorf("lanchat: opening discovery socket: %w", err)
	}
	tcpLn, err := listenTCP()
	if err != nil {
		udp.Close()
		return nil, fmt.Errorf("lanchat: opening transport listener: %w", err)
	}

	s := &Service{
		priv:        priv,
		pub:         pub,
		self:        peerIDFromPublicKey(pub),
		dataDir:     dataDir,
		displayName: loadDisplayName(dataDir),
		peers:       make(map[PeerID]*peerState),
		conns:       make(map[PeerID]*peerConn),
		rooms:       make(map[RoomID]*roomState),
		rawConns:    make(map[net.Conn]struct{}),
		udpConn:     udp,
		tcpLn:       tcpLn,
		tcpPort:     tcpLn.Addr().(*net.TCPAddr).Port,
		stopCh:      make(chan struct{}),
		notify:      notifySvc,
	}
	s.loadRooms()

	s.wg.Add(4)
	go s.broadcastLoop()
	go s.listenLoop()
	go s.expireLoop()
	go s.acceptLoop()

	return s, nil
}

// Close stops all background goroutines and releases the UDP/TCP
// sockets and any live peer connections — safe to call once; a second
// call is a no-op (stopOnce).
func (s *Service) Close() error {
	s.stopOnce.Do(func() {
		// close(stopCh) happens inside the same critical section as the
		// rawConns snapshot below (both guarded by mu) — that's what
		// closes the registration race in handleConn: a connection either
		// finishes registering into rawConns strictly before this lock is
		// taken (so it's in the snapshot) or strictly after stopCh is
		// already closed (so handleConn's own stopCh check catches it and
		// closes it immediately instead of registering). There's no
		// window where a connection could register into rawConns without
		// either this snapshot or handleConn's own check seeing it.
		s.mu.Lock()
		close(s.stopCh)
		s.udpConn.Close()
		s.tcpLn.Close()
		conns := make([]net.Conn, 0, len(s.rawConns))
		for nc := range s.rawConns {
			conns = append(conns, nc)
		}
		s.mu.Unlock()
		for _, nc := range conns {
			nc.Close()
		}
		s.wg.Wait()
	})
	return nil
}

// DialSeed connects directly to addr ("host:port" of another instance's
// TCP transport port), bypassing UDP discovery entirely. This is the
// escape hatch for segmented networks where broadcast can't reach a
// peer (routed subnets, some VPN/container setups) — and it's what
// integration tests use to connect two instances deterministically
// instead of depending on broadcast delivery timing.
func (s *Service) DialSeed(addr string) error {
	nc, err := net.DialTimeout("tcp4", addr, 5*time.Second)
	if err != nil {
		return err
	}
	s.wg.Add(1)
	go s.handleConn(nc, true)
	return nil
}

// TCPPort returns the OS-assigned port this instance's TCP transport
// listener is bound to (host is always all-interfaces — the caller
// knows, or looks up, which address is reachable) — combine with a
// reachable IP (e.g. "127.0.0.1" for same-host tests) to build the
// "host:port" DialSeed expects.
func (s *Service) TCPPort() int {
	return s.tcpPort
}

// SetDisplayName sets (or changes) this instance's display name,
// persists it, and re-announces immediately so peers see the change
// without waiting for the next beacon interval.
func (s *Service) SetDisplayName(name string) error {
	s.mu.Lock()
	s.displayName = name
	dataDir := s.dataDir
	s.mu.Unlock()
	if err := saveDisplayName(dataDir, name); err != nil {
		return err
	}
	s.sendBeacon()
	return nil
}

// RegenerateIdentity discards the current keypair and generates a new
// one — see the doc comment on loadOrCreateIdentity for why this is a
// deliberate "become a new identity" operation, not a key rotation that
// preserves continuity.
func (s *Service) RegenerateIdentity() error {
	return s.regenerateIdentity()
}

// Rooms returns a snapshot of every currently-known room (joined or
// merely announced), sorted by name for stable UI rendering.
func (s *Service) Rooms() []RoomSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RoomSummary, 0, len(s.rooms))
	for _, r := range s.rooms {
		if !r.known {
			continue
		}
		out = append(out, r.RoomSummary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Messages returns a snapshot of room's current message history (newest
// last), or nil if the room is unknown.
func (s *Service) Messages(room RoomID) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rooms[room]
	if !ok {
		return nil
	}
	return append([]Message{}, r.messages...)
}

// Peers returns a snapshot of every known LAN peer, online or not.
func (s *Service) Peers() []PeerSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PeerSummary, 0, len(s.peers))
	for id, ps := range s.peers {
		out = append(out, PeerSummary{ID: id, Name: ps.name, Online: ps.online})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
