package public

import _ "embed"

// DefaultAvatar contains the raw bytes of the fallback avatar image.
// It is embedded directly into the binary at compile time.
//
//go:embed avatar.jpg
var DefaultAvatar []byte
