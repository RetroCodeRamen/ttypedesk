package lanchat

import (
	"fmt"
	"testing"
	"time"
)

// TestEndToEndDiscoveryRoomHistoryConvergence is the closest thing to
// the plan's "two real machines" manual check that can run in an
// automated test: three real Service instances, connected to each
// other *only* via real UDP broadcast discovery (never DialSeed) —
// exercising the actual code path a real deployment uses end to end:
// discover, create a room, exchange messages, and have a peer that
// joins late converge on the room's full prior history.
func TestEndToEndDiscoveryRoomHistoryConvergence(t *testing.T) {
	alice := newTestService(t, "Alice")
	bob := newTestService(t, "Bob")

	aliceID, _ := alice.Self()
	bobID, _ := bob.Self()

	// 1. Discovery: each sees the other online, with no seeding of any
	// kind — purely from broadcast beacons received over real sockets.
	waitFor(t, 10*time.Second, "Alice sees Bob online", func() bool {
		for _, p := range alice.Peers() {
			if p.ID == bobID && p.Online {
				return true
			}
		}
		return false
	})
	waitFor(t, 10*time.Second, "Bob sees Alice online", func() bool {
		for _, p := range bob.Peers() {
			if p.ID == aliceID && p.Online {
				return true
			}
		}
		return false
	})

	// 2. Discovering each other should have driven the two of them to
	// actually connect (the smaller-PeerID-dials rule), with no test
	// helper forcing it — this is the real connectIfNeeded path.
	waitFor(t, 10*time.Second, "Alice and Bob have a live connection", func() bool {
		alice.mu.Lock()
		_, ok := alice.conns[bobID]
		alice.mu.Unlock()
		return ok
	})

	// 3. Room creation + messaging, still over that same organically-
	// established connection.
	room := alice.CreateRoom("Kitchen Table")
	waitFor(t, 5*time.Second, "Bob learns about the room", func() bool {
		for _, r := range bob.Rooms() {
			if r.ID == room {
				return true
			}
		}
		return false
	})
	bob.JoinRoom(room)

	for i := 0; i < 5; i++ {
		if err := alice.SendMessage(room, fmt.Sprintf("message %d", i)); err != nil {
			t.Fatalf("SendMessage(%d) = %v", i, err)
		}
	}
	waitFor(t, 5*time.Second, "Bob receives all 5 messages", func() bool {
		return len(bob.Messages(room)) == 5
	})
	if err := bob.SendMessage(room, "reply from bob"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "Alice receives Bob's reply", func() bool {
		for _, m := range alice.Messages(room) {
			if m.Body == "reply from bob" {
				return true
			}
		}
		return false
	})

	// 4. A late joiner — Carol, discovered and connected purely via
	// broadcast, same as Alice/Bob — converges on the full history
	// (6 messages: Alice's 5 plus Bob's reply) despite never having
	// been present for any of it, via history_sync alone.
	carol := newTestService(t, "Carol")
	carolID, _ := carol.Self()

	waitFor(t, 10*time.Second, "Carol discovers and connects to Alice", func() bool {
		alice.mu.Lock()
		_, ok := alice.conns[carolID]
		alice.mu.Unlock()
		return ok
	})
	waitFor(t, 10*time.Second, "Carol learns about the room", func() bool {
		for _, r := range carol.Rooms() {
			if r.ID == room {
				return true
			}
		}
		return false
	})
	carol.JoinRoom(room)

	waitFor(t, 5*time.Second, "Carol converges on the full 6-message history", func() bool {
		msgs := carol.Messages(room)
		if len(msgs) != 6 {
			return false
		}
		bodies := map[string]bool{}
		for _, m := range msgs {
			bodies[m.Body] = true
		}
		if !bodies["reply from bob"] {
			return false
		}
		for i := 0; i < 5; i++ {
			if !bodies[fmt.Sprintf("message %d", i)] {
				return false
			}
		}
		return true
	})
}
