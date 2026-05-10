package util

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// GenerateShortID returns an 8-character uppercase hex string (e.g. "A3F72C1D").
// Used as a human-readable unique identifier for courses, etc.
func GenerateShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("shortid: crypto/rand unavailable")
	}
	return strings.ToUpper(hex.EncodeToString(b))
}
