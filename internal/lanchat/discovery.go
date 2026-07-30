package lanchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// discoveryPort is the fixed, well-known UDP port every lanchat instance
// on the LAN listens on for beacons — has to be agreed in advance, since
// a peer doesn't know where to broadcast/listen otherwise (unlike the
// TCP transport port, which is OS-assigned and only ever learned
// dynamically, from inside a beacon).
const discoveryPort = 51888

// beaconInterval is how often this instance announces itself.
const beaconInterval = 5 * time.Second

// peerTimeout is how long a peer can go without a beacon before it's
// considered offline (comfortably longer than beaconInterval to absorb
// a couple of dropped/delayed broadcasts without flapping).
const peerTimeout = 30 * time.Second

// beacon is the UDP broadcast payload — deliberately tiny, sent
// frequently, on an unreliable transport (broadcast UDP), so there's no
// history or room info in it, just enough to find and dial a peer.
type beacon struct {
	PeerID  PeerID `json:"peer_id"`
	Name    string `json:"name"`
	TCPPort int    `json:"tcp_port"`
}

// udpConn is the subset of *net.UDPConn discovery.go needs — narrowed to
// an interface so tests can substitute a fake if loopback broadcast ever
// proves unreliable in CI.
type udpConn interface {
	WriteToUDP(b []byte, addr *net.UDPAddr) (int, error)
	ReadFromUDP(b []byte) (int, *net.UDPAddr, error)
	Close() error
	SetReadDeadline(t time.Time) error
}

// listenUDP opens the shared discovery socket: bound to
// 0.0.0.0:discoveryPort so it receives broadcasts from any interface,
// with SO_BROADCAST set so it's also allowed to send them, and
// SO_REUSEADDR/SO_REUSEPORT set (via ListenConfig.Control, which runs
// after socket() but before bind()) so more than one lanchat instance
// can run on the same host at once — without this, a second instance
// (or, in-process, this package's own two-Service tests) would fail to
// even start with "address already in use", and on Linux SO_REUSEPORT
// specifically is what makes the kernel deliver a copy of each incoming
// broadcast datagram to every such socket, not just one of them.
func listenUDP() (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
					sockErr = err
					return
				}
				sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}
			return sockErr
		},
	}
	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", discoveryPort))
	if err != nil {
		return nil, err
	}
	conn := pc.(*net.UDPConn)
	if err := setBroadcast(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// setBroadcast sets SO_BROADCAST on conn via SyscallConn — the standard
// library has no portable way to set this option directly, but
// golang.org/x/sys/unix (already a direct dependency) does the raw
// setsockopt call, and SyscallConn gives safe access to the underlying
// fd without breaking net.UDPConn's own management of it.
func setBroadcast(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
	})
	if err != nil {
		return err
	}
	return sockErr
}

// broadcastLoop sends this instance's beacon to every broadcast address
// on the local machine's interfaces every beaconInterval, until stopCh
// closes.
func (s *Service) broadcastLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(beaconInterval)
	defer ticker.Stop()

	s.sendBeacon()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.sendBeacon()
		}
	}
}

func (s *Service) sendBeacon() {
	s.mu.Lock()
	b := beacon{PeerID: s.self, Name: s.displayName, TCPPort: s.tcpPort}
	conn := s.udpConn
	s.mu.Unlock()
	if conn == nil {
		return
	}
	data, err := json.Marshal(b)
	if err != nil {
		return
	}
	for _, addr := range broadcastAddrs() {
		conn.WriteToUDP(data, &net.UDPAddr{IP: addr, Port: discoveryPort})
	}
}

// broadcastAddrs computes the IPv4 broadcast address of every up,
// non-loopback interface (e.g. 192.168.1.10/24 → 192.168.1.255), plus
// 255.255.255.255 as a catch-all, plus loopback's own broadcast so a
// same-host peer (real-world multi-instance testing, or this package's
// own tests) is reachable too.
func broadcastAddrs() []net.IP {
	addrs := []net.IP{net.IPv4bcast}
	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		ifAddrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifAddrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			bcast := make(net.IP, len(ip4))
			for i := range ip4 {
				bcast[i] = ip4[i] | ^ipNet.Mask[i]
			}
			addrs = append(addrs, bcast)
		}
	}
	return addrs
}

// listenLoop reads beacons off the shared UDP socket and applies them,
// until stopCh closes.
func (s *Service) listenLoop() {
	defer s.wg.Done()
	buf := make([]byte, 2048)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		s.mu.Lock()
		conn := s.udpConn
		s.mu.Unlock()
		if conn == nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-s.stopCh:
				return
			default:
				continue
			}
		}
		var b beacon
		if err := json.Unmarshal(buf[:n], &b); err != nil {
			continue
		}
		s.applyBeacon(b, src.IP)
	}
}

// applyBeacon records/refreshes a peer from a received beacon and, on a
// genuinely new peer or one transitioning offline→online, connects to
// it (if we're the dialing side — see connectIfNeeded) and emits
// EventPeerOnline.
func (s *Service) applyBeacon(b beacon, srcIP net.IP) {
	if b.PeerID == "" || b.PeerID == s.selfID() {
		return
	}
	addr := net.JoinHostPort(srcIP.String(), strconv.Itoa(b.TCPPort))

	s.mu.Lock()
	ps, existed := s.peers[b.PeerID]
	wasOnline := existed && ps.online
	if !existed {
		ps = &peerState{}
		s.peers[b.PeerID] = ps
	}
	ps.name = b.Name
	ps.addr = addr
	ps.lastSeen = time.Now()
	ps.online = true
	s.mu.Unlock()

	if !wasOnline {
		s.emit(Event{Kind: EventPeerOnline, Peer: b.PeerID})
		s.connectIfNeeded(b.PeerID)
	}
}

// expireLoop periodically drops peers not heard from within peerTimeout,
// firing EventPeerOffline for each.
func (s *Service) expireLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(peerTimeout / 3)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.expirePeers()
		}
	}
}

func (s *Service) expirePeers() {
	now := time.Now()
	var offline []PeerID
	s.mu.Lock()
	for id, ps := range s.peers {
		if ps.online && now.Sub(ps.lastSeen) > peerTimeout {
			ps.online = false
			offline = append(offline, id)
		}
	}
	s.mu.Unlock()
	for _, id := range offline {
		s.emit(Event{Kind: EventPeerOffline, Peer: id})
	}
}
