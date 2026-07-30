package lanchat

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// wireType identifies a TCP protocol message (see doc comment at the top
// of transport.go for the full message flow). NDJSON framing — one
// compact JSON object per line — matching this project's other
// out-of-process wire protocol (see docs/extapp.md): messages here are
// small and infrequent, no reason to reach for binary framing.
type wireType string

const (
	wireHello        wireType = "hello"
	wireRoomAnnounce wireType = "room_announce"
	wireRoomJoin     wireType = "room_join"
	wireHistorySync  wireType = "history_sync"
	wireMessage      wireType = "message"
)

// wireEnvelope wraps every TCP protocol message.
type wireEnvelope struct {
	Type    wireType        `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func encodeWire(typ wireType, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(wireEnvelope{Type: typ, Payload: raw})
}

func decodeWire(data []byte) (wireEnvelope, error) {
	var env wireEnvelope
	err := json.Unmarshal(data, &env)
	return env, err
}

func decodeWirePayload[T any](env wireEnvelope) (T, error) {
	var v T
	if len(env.Payload) == 0 {
		return v, nil
	}
	err := json.Unmarshal(env.Payload, &v)
	return v, err
}

// wireHelloMsg is sent immediately by both sides right after a TCP
// connection is established — introduces the sender before anything else
// is exchanged over it.
type wireHelloMsg struct {
	PeerID PeerID `json:"peer_id"`
	Name   string `json:"name"`
}

// wireRoomAnnounceMsg gossips a room's existence (never sent for a DM
// room — those aren't discoverable, only ever known to their two
// participants). Receiving one doesn't join the room, just makes it
// visible as something that could be joined.
type wireRoomAnnounceMsg struct {
	RoomID    RoomID `json:"room_id"`
	Name      string `json:"name"`
	CreatedBy PeerID `json:"created_by"`
	CreatedAt int64  `json:"created_at"` // unix nanoseconds
}

// wireRoomJoinMsg signals "PeerID is a member of this room, send it
// history and include it in future messages for it." Sent for every
// room the sender has personally joined, to every currently-connected
// peer (including DM rooms — see docs comment on fan-out in rooms.go for
// why broadcasting a DM's room_join doesn't leak its content), and
// re-gossiped onward unchanged by relays so it reaches peers beyond a
// direct connection. PeerID is carried explicitly (rather than inferred
// from which connection a copy arrives over) precisely because of that
// relaying: a relay forwarding someone else's join must not cause the
// relay itself to be mistaken for a member — see applyRoomJoin.
type wireRoomJoinMsg struct {
	RoomID RoomID `json:"room_id"`
	PeerID PeerID `json:"peer_id"`
}

// wireHistorySyncMsg answers a wireRoomJoinMsg with everything the
// replying peer has for that room (possibly nothing, if it's unknown to
// them too).
type wireHistorySyncMsg struct {
	RoomID   RoomID    `json:"room_id"`
	Messages []Message `json:"messages"`
}

// wireMessageMsg carries one new chat message — see signMessage/
// verifyMessage for how Sig is produced/checked.
type wireMessageMsg struct {
	RoomID RoomID  `json:"room_id"`
	Msg    Message `json:"msg"`
}

// signingBytes returns the canonical byte sequence a message's
// signature covers — every field that matters for authenticity, in a
// fixed order, so both sides always compute identical bytes.
func signingBytes(room RoomID, sender PeerID, senderName string, body string, t int64) []byte {
	return []byte(string(room) + "\x00" + string(sender) + "\x00" + senderName + "\x00" + body + "\x00" + strconv.FormatInt(t, 10))
}

// messageID derives a message's dedup key from its content — two peers
// that independently receive the same message (via different gossip
// paths) compute the same ID, so re-delivery is a harmless no-op instead
// of a duplicate.
func messageID(room RoomID, sender PeerID, t int64, body string) string {
	sum := sha256.Sum256(signingBytes(room, sender, "", body, t))
	return hex.EncodeToString(sum[:])
}

// signMessage signs a new outgoing message with priv, producing a
// complete Message (ID, Time, and Sig all filled in). room is bound into
// the signature so a captured message can't be replayed into a
// different room with a forged room_id.
func signMessage(priv ed25519.PrivateKey, self PeerID, selfName string, room RoomID, body string, t time.Time) Message {
	ts := t.UnixNano()
	sig := ed25519.Sign(priv, signingBytes(room, self, selfName, body, ts))
	return Message{
		ID:         messageID(room, self, ts, body),
		Sender:     self,
		SenderName: selfName,
		Body:       body,
		Time:       t,
		Sig:        sig,
	}
}

// verifyMessage checks m's signature against its claimed sender's public
// key (derived directly from the PeerID — no separate key-lookup step,
// since the ID *is* the key) for the room it arrived in — room is
// supplied by the caller (the wire envelope it came in on), not trusted
// from within m itself.
func verifyMessage(room RoomID, m Message) error {
	pub, err := m.Sender.publicKey()
	if err != nil {
		return err
	}
	data := signingBytes(room, m.Sender, m.SenderName, m.Body, m.Time.UnixNano())
	if !ed25519.Verify(pub, data, m.Sig) {
		return fmt.Errorf("lanchat: signature verification failed for message %s", m.ID)
	}
	return nil
}
