package lanchat

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
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
// with no '\n'. It does *not* infer success from a write() error: TCP
// send-side buffering means Write can keep "succeeding" locally for a
// while even after the remote end has already closed its side (the
// kernel absorbs it into the socket buffer regardless of whether
// anything's still reading), so that's not a reliable signal here. It
// verifies server-side closure directly instead — a Read on the same
// connection surfacing EOF/reset well within a bounded deadline. If the
// read is unbounded, no close ever happens and a Read attempt just
// blocks until the deadline, which is a distinguishable timeout error.
func TestUnterminatedLineDoesNotGrowMemoryUnbounded(t *testing.T) {
	svc := newTestService(t, "Alice")

	nc, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", svc.TCPPort()), 5*time.Second)
	if err != nil {
		t.Fatalf("Dial() = %v", err)
	}
	defer nc.Close()

	// handleConn sends its own hello line immediately on accept, before
	// ever reading anything from us — drain and discard it first so the
	// later "did the server close on us" read isn't just picking that up
	// instead.
	nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	helloBuf := bufio.NewReader(nc)
	if _, err := helloBuf.ReadBytes('\n'); err != nil {
		t.Fatalf("reading server's initial hello: %v", err)
	}

	chunk := make([]byte, 64*1024)
	for i := range chunk {
		chunk[i] = 'A'
	}
	sent := 0
	target := maxWireLineBytes + 4*1024*1024 // comfortably past the cap
	nc.SetWriteDeadline(time.Now().Add(10 * time.Second))
	for sent < target {
		n, werr := nc.Write(chunk)
		sent += n
		if werr != nil {
			break // the remote side is gone; nothing more to write
		}
	}

	nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, rerr := helloBuf.Read(make([]byte, 16)) // same reader as the hello drain — don't skip its internal buffer
	if rerr == nil {
		t.Fatal("server sent data back unexpectedly")
	}
	var ne net.Error
	if errors.As(rerr, &ne) && ne.Timeout() {
		t.Fatalf("server never closed the connection after %d bytes of unterminated data (read timed out instead of observing a close) — the read is unbounded again", sent)
	}
	// Any other error (EOF, connection reset, ...) confirms the server
	// actually closed its side once the cap was exceeded — the expected,
	// correct outcome.
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
