package assets

import _ "embed"

// Embedded wallpapers (JPEG). Config paths use builtin:<id>.

//go:embed wallpapers/bliss.jpg
var Bliss []byte

//go:embed wallpapers/scarlet.jpg
var Scarlet []byte

//go:embed wallpapers/bumble.jpg
var Bumble []byte

//go:embed wallpapers/bubble.jpg
var Bubble []byte

//go:embed wallpapers/sprout.jpg
var Sprout []byte

const (
	BuiltinBliss   = "builtin:bliss"
	BuiltinScarlet = "builtin:scarlet"
	BuiltinBumble  = "builtin:bumble"
	BuiltinBubble  = "builtin:bubble"
	BuiltinSprout  = "builtin:sprout"
)

// BuiltinBytes returns embedded wallpaper bytes for a builtin: path, or nil.
func BuiltinBytes(path string) []byte {
	switch path {
	case BuiltinBliss, "":
		return Bliss
	case BuiltinScarlet:
		return Scarlet
	case BuiltinBumble:
		return Bumble
	case BuiltinBubble:
		return Bubble
	case BuiltinSprout:
		return Sprout
	default:
		return nil
	}
}

// IsBuiltin reports whether path is an embedded wallpaper token.
func IsBuiltin(path string) bool {
	return BuiltinBytes(path) != nil || path == ""
}
