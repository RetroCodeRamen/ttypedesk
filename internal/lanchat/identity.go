package lanchat

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// identityFileName is where a Service's Ed25519 keypair is persisted —
// deliberately its own file under the Service's dataDir rather than
// internal/credstore: credstore has a single fixed, unparameterized
// location (~/.config/ttypedesk/credentials/), one per real user
// account, with no notion of "which instance." That's fine for the
// single-instance production case, but it means two Services in one
// process (this package's own multi-instance tests, or in principle two
// ttypedesk processes on one host) would silently load/generate the
// *same* keypair and believe they're the same peer — and every test run
// would overwrite the real user's actual saved identity as a side
// effect. Keeping it inside dataDir keeps identity storage scoped
// exactly like everything else the Service owns (rooms, profile), 0600,
// never in config.json or git, just not routed through credstore.
const identityFileName = "identity.key"

func identityPath(dataDir string) string {
	return filepath.Join(dataDir, identityFileName)
}

// loadOrCreateIdentity loads a previously-generated Ed25519 keypair from
// dataDir, or generates and persists a new one on first run. The
// keypair — not the display name — is the real, permanent identity:
// regenerating it (Settings → LAN Chat → Regenerate Identity) starts a
// new identity from everyone else's perspective, on purpose, since
// there's no central authority to reassign an old identity to a new key.
func loadOrCreateIdentity(dataDir string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(identityPath(dataDir))
	switch {
	case err == nil:
		if priv, perr := parsePrivateKey(raw); perr == nil {
			return priv.Public().(ed25519.PublicKey), priv, nil
		}
		// Corrupt/unexpected-length data under this key — fall through
		// and generate a fresh one rather than failing Service startup
		// entirely over a stale/damaged file.
	case errors.Is(err, os.ErrNotExist):
		// Nothing saved yet — the expected first-run case, fall through.
	default:
		// A real error (permissions, disk) reading an existing key is
		// worth surfacing rather than silently minting a new identity
		// out from under the user.
		return nil, nil, fmt.Errorf("lanchat: loading identity: %w", err)
	}
	return generateAndSaveIdentity(dataDir)
}

func generateAndSaveIdentity(dataDir string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("lanchat: generating identity: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("lanchat: saving identity: %w", err)
	}
	if err := os.WriteFile(identityPath(dataDir), priv, 0o600); err != nil {
		return nil, nil, fmt.Errorf("lanchat: saving identity: %w", err)
	}
	return pub, priv, nil
}

// regenerateIdentity discards the current keypair and generates a new
// one, saving it over the old identity file.
func (s *Service) regenerateIdentity() error {
	s.mu.Lock()
	dataDir := s.dataDir
	s.mu.Unlock()

	pub, priv, err := generateAndSaveIdentity(dataDir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.priv, s.pub, s.self = priv, pub, peerIDFromPublicKey(pub)
	s.mu.Unlock()
	return nil
}

func peerIDFromPublicKey(pub ed25519.PublicKey) PeerID {
	return PeerID(hex.EncodeToString(pub))
}

func (p PeerID) publicKey() (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(string(p))
	if err != nil {
		return nil, fmt.Errorf("lanchat: invalid peer id %q: %w", p, err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("lanchat: peer id %q is %d bytes, want %d", p, len(b), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(b), nil
}

func parsePrivateKey(raw []byte) (ed25519.PrivateKey, error) {
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("lanchat: identity is %d bytes, want %d", len(raw), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(raw), nil
}

// profile is a display name persisted alongside room data (not
// credstore — a display name isn't a secret, and unlike the keypair
// it's meant to be user-editable, which is exactly what apps/settings'
// LAN Chat page will do via SetDisplayName). Kept as its own tiny file
// rather than added to config.Config: it's service-owned identity state,
// not desktop-wide configuration, so lanchat.Service can stay
// self-contained without server.go needing to plumb config writes
// through it.
type profile struct {
	DisplayName string `json:"display_name"`
}

func profilePath(dataDir string) string {
	return filepath.Join(dataDir, "profile.json")
}

// loadDisplayName returns the persisted display name, or "" if none has
// been set yet (the first-run "pick a name" case).
func loadDisplayName(dataDir string) string {
	data, err := os.ReadFile(profilePath(dataDir))
	if err != nil {
		return ""
	}
	var p profile
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.DisplayName
}

func saveDisplayName(dataDir, name string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile{DisplayName: name}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(profilePath(dataDir), data, 0o600)
}
