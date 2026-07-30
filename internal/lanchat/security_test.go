package lanchat

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// TestUnterminatedLineDoesNotGrowMemoryUnbounded is a regression test for
// a real unbounded-memory DoS: handleConn used to read with
// bufio.Reader.ReadBytes('\n'), which has no maximum line length and
// grows its internal buffer forever as long as bytes keep arriving with
// no '\n'. A peer (any connected peer — nothing here requires
// authentication before connecting) that just streams data with no
// newline could grow the receiving process's memory without bound.
//
// This connects directly to a real Service's TCP listener (bypassing the
// hello handshake entirely — the bug is in the raw read loop, which runs
// before handshake completion too) and streams well past maxWireLineBytes
// with no '\n', then confirms the connection gets closed (Scan() fails)
// rather than the read continuing to buffer forever.
func TestUnterminatedLineDoesNotGrowMemoryUnbounded(t *testing.T) {
	svc := newTestService(t, "Alice")

	nc, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", svc.TCPPort()), 5*time.Second)
	if err != nil {
		t.Fatalf("Dial() = %v", err)
	}
	defer nc.Close()

	// Stream well past the cap with no '\n' — if handleConn's read is
	// truly unbounded, nothing here would ever return; the connection
	// would just keep accepting bytes and growing its buffer forever.
	chunk := make([]byte, 64*1024)
	for i := range chunk {
		chunk[i] = 'A'
	}
	sent := 0
	nc.SetWriteDeadline(time.Now().Add(10 * time.Second))
	for sent < maxWireLineBytes+4*1024*1024 { // comfortably past the cap
		n, err := nc.Write(chunk)
		sent += n
		if err != nil {
			// The other end closed the connection once the cap was
			// exceeded — exactly the expected outcome, not a test
			// failure. A real unbounded reader would accept all of this
			// without ever erroring.
			return
		}
	}

	// If we got here, the server accepted more than maxWireLineBytes of
	// unterminated data without ever closing the connection — the bug
	// this test guards against.
	t.Fatalf("wrote %d bytes (> maxWireLineBytes=%d) of unterminated data with no error — the read is unbounded again", sent, maxWireLineBytes)
}

// TestSendMessageRejectsOverlongBody guards SendMessage's own length
// check — the source-side half of the fix (the ingest-side half is
// TestIngestMessageRejectsOverlongBodyEvenWithValidSignature below).
func TestSendMessageRejectsOverlongBody(t *testing.T) {
	svc := newTestService(t, "Alice")
	room := svc.CreateRoom("General")

	huge := strings.Repeat("x", maxMessageBodyRunes+1)
	if err := svc.SendMessage(room, huge); err == nil {
		t.Fatal("SendMessage() with an over-length body = nil error, want a rejection")
	}

	ok := strings.Repeat("x", maxMessageBodyRunes)
	if err := svc.SendMessage(room, ok); err != nil {
		t.Fatalf("SendMessage() at exactly the limit = %v, want success", err)
	}
}

// TestIngestMessageRejectsOverlongBodyEvenWithValidSignature confirms the
// defense-in-depth check in ingestMessage: a modified/malicious peer
// isn't bound by our own SendMessage validation, so a genuinely
// well-signed but over-length message must still be dropped on receipt.
func TestIngestMessageRejectsOverlongBodyEvenWithValidSignature(t *testing.T) {
	svc := newTestService(t, "Alice")
	room := svc.CreateRoom("General")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sender := peerIDFromPublicKey(pub)
	huge := strings.Repeat("x", maxMessageBodyRunes+1)
	m := signMessage(priv, sender, "Mallory", room, huge, time.Now())

	if ok := svc.ingestMessage(room, m); ok {
		t.Fatal("ingestMessage() accepted a validly-signed but over-length message, want rejection")
	}
}

// TestApplyHistorySyncCapsMessageCount guards against a peer packing far
// more than maxRoomMessages into a single history_sync to force
// excessive signature-verification work on the receiver.
func TestApplyHistorySyncCapsMessageCount(t *testing.T) {
	svc := newTestService(t, "Alice")
	room := RoomID("some-room")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sender := peerIDFromPublicKey(pub)

	var msgs []Message
	base := time.Now()
	for i := 0; i < maxRoomMessages*3; i++ {
		msgs = append(msgs, signMessage(priv, sender, "Bob", room, "m", base.Add(time.Duration(i)*time.Millisecond)))
	}

	svc.applyHistorySync(wireHistorySyncMsg{RoomID: room, Messages: msgs})

	got := svc.Messages(room)
	if len(got) > maxRoomMessages {
		t.Fatalf("Messages() len = %d after an oversized history_sync, want <= %d", len(got), maxRoomMessages)
	}
}
