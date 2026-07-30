package lanchat

import (
	"fmt"
	"testing"
	"time"
)

// newTestService starts a real Service with a fresh identity and an
// isolated temp data directory — a real UDP+TCP-backed instance, not a
// mock, per this project's established testing bar.
func newTestService(t *testing.T, name string) *Service {
	t.Helper()
	dir := t.TempDir()
	svc, err := New(dir, nil)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	if err := svc.SetDisplayName(name); err != nil {
		t.Fatalf("SetDisplayName() = %v", err)
	}
	return svc
}

// waitFor polls cond every 10ms until it's true or timeout elapses,
// failing the test otherwise — used throughout instead of a fixed sleep
// since gossip delivery is asynchronous.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", desc)
}

// connect seeds a and b together directly (bypassing UDP discovery
// timing, see DialSeed's doc comment) and waits for both sides to
// register each other as connected peers.
func connect(t *testing.T, a, b *Service) {
	t.Helper()
	if err := a.DialSeed(fmt.Sprintf("127.0.0.1:%d", b.TCPPort())); err != nil {
		t.Fatalf("DialSeed() = %v", err)
	}
	bID, _ := b.Self()
	aID, _ := a.Self()
	waitFor(t, 5*time.Second, "a connected to b", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		_, ok := a.conns[bID]
		return ok
	})
	waitFor(t, 5*time.Second, "b connected to a", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		_, ok := b.conns[aID]
		return ok
	})
}

func TestTwoServicesDiscoverEachOtherViaBroadcast(t *testing.T) {
	a := newTestService(t, "Alice")
	b := newTestService(t, "Bob")

	aID, _ := a.Self()
	bID, _ := b.Self()

	waitFor(t, 10*time.Second, "a sees b online via UDP broadcast", func() bool {
		for _, p := range a.Peers() {
			if p.ID == bID && p.Online {
				return true
			}
		}
		return false
	})
	waitFor(t, 10*time.Second, "b sees a online via UDP broadcast", func() bool {
		for _, p := range b.Peers() {
			if p.ID == aID && p.Online {
				return true
			}
		}
		return false
	})
}

func TestSendMessageDeliversToConnectedPeer(t *testing.T) {
	a := newTestService(t, "Alice")
	b := newTestService(t, "Bob")
	connect(t, a, b)

	room := a.CreateRoom("General")
	b.JoinRoom(room)

	// b's JoinRoom races the room_announce a already sent it (via
	// CreateRoom, over the connection established by connect); wait
	// until b actually knows about the room before joining, otherwise
	// JoinRoom is a silent no-op on an unknown room ID.
	waitFor(t, 5*time.Second, "b learns about the room", func() bool {
		for _, r := range b.Rooms() {
			if r.ID == room {
				return true
			}
		}
		return false
	})
	b.JoinRoom(room)

	if err := a.SendMessage(room, "hello from alice"); err != nil {
		t.Fatalf("SendMessage() = %v", err)
	}

	waitFor(t, 5*time.Second, "b receives the message", func() bool {
		for _, m := range b.Messages(room) {
			if m.Body == "hello from alice" {
				return true
			}
		}
		return false
	})
}

func TestNewJoinerConvergesOnExistingHistory(t *testing.T) {
	a := newTestService(t, "Alice")
	b := newTestService(t, "Bob")
	connect(t, a, b)

	room := a.CreateRoom("General")
	if err := a.SendMessage(room, "message 1"); err != nil {
		t.Fatal(err)
	}
	if err := a.SendMessage(room, "message 2"); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 5*time.Second, "b learns about the room", func() bool {
		for _, r := range b.Rooms() {
			if r.ID == room {
				return true
			}
		}
		return false
	})

	// b joins after both messages were already sent — history_sync
	// (sent in reply to b's room_join) is what's supposed to bring it up
	// to date, not re-delivery of the original SendMessage calls.
	b.JoinRoom(room)

	waitFor(t, 3*time.Second, "b converges on both prior messages", func() bool {
		msgs := b.Messages(room)
		if len(msgs) != 2 {
			return false
		}
		bodies := map[string]bool{}
		for _, m := range msgs {
			bodies[m.Body] = true
		}
		return bodies["message 1"] && bodies["message 2"]
	})
}

func TestDMRoomIsPrivateToTheTwoParticipants(t *testing.T) {
	a := newTestService(t, "Alice")
	b := newTestService(t, "Bob")
	c := newTestService(t, "Carol")
	connect(t, a, b)
	connect(t, a, c)
	connect(t, b, c)

	bID, _ := b.Self()
	room := a.DMRoom(bID)

	// b starts its side of the DM the same way the real UI would (the
	// user clicking Alice's name) — not by passively waiting to
	// "discover" it via gossip, since a DM's room_join is deliberately
	// indistinguishable from noise to anyone who doesn't already know to
	// derive this exact room ID (see roomState.known's doc comment).
	// Both sides independently derive the identical room ID.
	aID, _ := a.Self()
	if got := b.DMRoom(aID); got != room {
		t.Fatalf("b.DMRoom(a) = %q, want %q (same as a.DMRoom(b))", got, room)
	}

	if err := a.SendMessage(room, "just for you"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, "b receives the DM", func() bool {
		for _, m := range b.Messages(room) {
			if m.Body == "just for you" {
				return true
			}
		}
		return false
	})

	// Carol was never told this room's ID by anyone and can't derive it
	// (she doesn't know it's "a's DM with b" instead of any other room),
	// so she should never see it appear in her known rooms.
	time.Sleep(300 * time.Millisecond)
	for _, r := range c.Rooms() {
		if r.ID == room {
			t.Fatalf("Carol knows about the private DM room %q — privacy leak", room)
		}
	}
}

func TestRoomHistoryCapAt500Messages(t *testing.T) {
	a := newTestService(t, "Alice")
	room := a.CreateRoom("Busy")
	for i := 0; i < 510; i++ {
		if err := a.SendMessage(room, fmt.Sprintf("msg %d", i)); err != nil {
			t.Fatalf("SendMessage(%d) = %v", i, err)
		}
	}
	msgs := a.Messages(room)
	if len(msgs) != maxRoomMessages {
		t.Fatalf("len(Messages()) = %d, want %d", len(msgs), maxRoomMessages)
	}
	if msgs[len(msgs)-1].Body != "msg 509" {
		t.Fatalf("newest message = %q, want %q", msgs[len(msgs)-1].Body, "msg 509")
	}
}

func TestRoomPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDisplayName("Alice"); err != nil {
		t.Fatal(err)
	}
	room := svc.CreateRoom("General")
	if err := svc.SendMessage(room, "persisted message"); err != nil {
		t.Fatal(err)
	}
	svc.Close()

	svc2, err := New(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc2.Close()

	id2, name2 := svc2.Self()
	id1, _ := svc.Self()
	if id2 != id1 {
		t.Fatalf("identity not persisted: got %q, want %q", id2, id1)
	}
	if name2 != "Alice" {
		t.Fatalf("display name not persisted: got %q, want %q", name2, "Alice")
	}

	msgs := svc2.Messages(room)
	if len(msgs) != 1 || msgs[0].Body != "persisted message" {
		t.Fatalf("Messages() after restart = %+v, want the persisted message", msgs)
	}
}

// TestRegenerateIdentityConcurrentWithDiscoveryIsRace-free is a
// regression test for a real data race CI caught: RegenerateIdentity
// writes s.self under s.mu, but several other code paths (notably
// discovery.go's applyBeacon, hit continuously by a second real
// instance's beacons) used to read the bare field without the lock.
// Two real Services, real broadcast discovery between them, and
// RegenerateIdentity called repeatedly on one while the other keeps
// beaconing — this only proves anything under `go test -race`.
func TestRegenerateIdentityConcurrentWithDiscoveryIsRaceFree(t *testing.T) {
	a := newTestService(t, "Alice")
	b := newTestService(t, "Bob")

	bID, _ := b.Self()
	waitFor(t, 10*time.Second, "a discovers b", func() bool {
		for _, p := range a.Peers() {
			if p.ID == bID && p.Online {
				return true
			}
		}
		return false
	})

	for i := 0; i < 20; i++ {
		if err := a.RegenerateIdentity(); err != nil {
			t.Fatalf("RegenerateIdentity(%d) = %v", i, err)
		}
	}
}

func TestSendMessageFailsWhenNotJoined(t *testing.T) {
	a := newTestService(t, "Alice")
	if err := a.SendMessage(RoomID("unknown-room"), "hi"); err == nil {
		t.Fatal("SendMessage() on an unjoined room = nil, want an error")
	}
}

func TestSubscribeReceivesLocalMessageEvent(t *testing.T) {
	a := newTestService(t, "Alice")
	room := a.CreateRoom("General")

	events := make(chan Event, 4)
	a.Subscribe(func(ev Event) { events <- ev })

	if err := a.SendMessage(room, "hi"); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		if ev.Kind != EventMessage || ev.Msg.Body != "hi" {
			t.Fatalf("event = %+v, want EventMessage with body %q", ev, "hi")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventMessage")
	}
}
