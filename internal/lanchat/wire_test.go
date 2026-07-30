package lanchat

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestSignMessageAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	self := peerIDFromPublicKey(pub)
	room := RoomID("room-1")

	m := signMessage(priv, self, "Alice", room, "hello", time.Now())

	if err := verifyMessage(room, m); err != nil {
		t.Fatalf("verifyMessage() = %v, want nil", err)
	}
}

func TestVerifyMessageRejectsTamperedBody(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	self := peerIDFromPublicKey(pub)
	room := RoomID("room-1")

	m := signMessage(priv, self, "Alice", room, "hello", time.Now())
	m.Body = "tampered"

	if err := verifyMessage(room, m); err == nil {
		t.Fatal("verifyMessage() = nil for a tampered body, want an error")
	}
}

func TestVerifyMessageRejectsWrongRoom(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	self := peerIDFromPublicKey(pub)

	m := signMessage(priv, self, "Alice", RoomID("room-1"), "hello", time.Now())

	if err := verifyMessage(RoomID("room-2"), m); err == nil {
		t.Fatal("verifyMessage() = nil for a replayed-into-a-different-room message, want an error")
	}
}

func TestVerifyMessageRejectsForgedSender(t *testing.T) {
	_, priv1, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	room := RoomID("room-1")

	// Sign as priv1 but claim pub2's identity as the sender.
	m := signMessage(priv1, peerIDFromPublicKey(pub2), "Mallory", room, "hello", time.Now())

	if err := verifyMessage(room, m); err == nil {
		t.Fatal("verifyMessage() = nil for a forged sender, want an error")
	}
}

func TestMessageIDDeterministicForIdenticalContent(t *testing.T) {
	room := RoomID("room-1")
	sender := PeerID("abc")
	ts := time.Now().UnixNano()

	id1 := messageID(room, sender, ts, "hi")
	id2 := messageID(room, sender, ts, "hi")
	if id1 != id2 {
		t.Fatalf("messageID() not deterministic: %q vs %q", id1, id2)
	}

	id3 := messageID(room, sender, ts, "different")
	if id1 == id3 {
		t.Fatal("messageID() collided for different message bodies")
	}
}

func TestEncodeDecodeWireRoundTrip(t *testing.T) {
	orig := wireHelloMsg{PeerID: "abc", Name: "Alice"}
	data, err := encodeWire(wireHello, orig)
	if err != nil {
		t.Fatal(err)
	}
	env, err := decodeWire(data)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != wireHello {
		t.Fatalf("Type = %q, want %q", env.Type, wireHello)
	}
	got, err := decodeWirePayload[wireHelloMsg](env)
	if err != nil {
		t.Fatal(err)
	}
	if got != orig {
		t.Fatalf("round-tripped payload = %+v, want %+v", got, orig)
	}
}

func TestDMRoomIDOrderIndependent(t *testing.T) {
	a := PeerID("aaaa")
	b := PeerID("bbbb")
	if dmRoomID(a, b) != dmRoomID(b, a) {
		t.Fatal("dmRoomID(a, b) != dmRoomID(b, a), want order-independent")
	}
}

func TestRoomIDDiffersForDifferentCreatorsOrNames(t *testing.T) {
	ts := time.Now().UnixNano()
	id1 := roomID(PeerID("a"), "General", ts)
	id2 := roomID(PeerID("b"), "General", ts)
	id3 := roomID(PeerID("a"), "Other", ts)
	if id1 == id2 || id1 == id3 || id2 == id3 {
		t.Fatalf("roomID() collided: id1=%q id2=%q id3=%q", id1, id2, id3)
	}
}
