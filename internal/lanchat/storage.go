package lanchat

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// roomsDirName is the subdirectory of the Service's data directory each
// room's JSON file lives under — a dedicated directory, deliberately
// separate from internal/credstore (that's reserved for secrets; only
// the identity keypair uses it, see identity.go) since room history
// isn't a secret, just local state.
const roomsDirName = "rooms"

// persistedRoom is what actually gets written to
// <dataDir>/rooms/<room_id>.json — RoomSummary plus its capped message
// history. seen/subs are gossip bookkeeping, not persisted: seen is
// trivially rebuilt from the message list on load, and subs (who's
// currently listening) is inherently a live-connection concept that
// means nothing across a restart.
type persistedRoom struct {
	RoomSummary
	Messages []Message `json:"messages"`
}

func (s *Service) roomFilePath(id RoomID) string {
	return filepath.Join(s.dataDir, roomsDirName, string(id)+".json")
}

// persistRoom writes room id's current state to disk, atomically (write
// to a temp file in the same directory, then rename — never leaves a
// half-written file at the real path even if the process dies mid-write).
func (s *Service) persistRoom(id RoomID) {
	s.mu.Lock()
	r, ok := s.rooms[id]
	var pr persistedRoom
	if ok {
		pr = persistedRoom{RoomSummary: r.RoomSummary, Messages: append([]Message{}, r.messages...)}
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	dir := filepath.Join(s.dataDir, roomsDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "."+string(id)+".tmp-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	os.Rename(tmpPath, s.roomFilePath(id))
}

// loadRooms populates s.rooms from every persisted room file under the
// data directory — called once during New, before any network activity
// starts, so gossip immediately has real local history to offer new
// joiners.
func (s *Service) loadRooms() {
	dir := filepath.Join(s.dataDir, roomsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var pr persistedRoom
		if err := json.Unmarshal(data, &pr); err != nil {
			continue
		}
		seen := make(map[string]bool, len(pr.Messages))
		for _, m := range pr.Messages {
			seen[m.ID] = true
		}
		s.rooms[pr.ID] = &roomState{
			RoomSummary: pr.RoomSummary,
			messages:    pr.Messages,
			seen:        seen,
			subs:        map[PeerID]bool{},
			known:       true,
		}
	}
}
