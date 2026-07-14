package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// GenerateOTP returns a 6-digit numeric one-time code (e.g. "042913"),
// zero-padded, using crypto/rand — same randomness source as GenerateShortID.
func GenerateOTP() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		panic("otp: crypto/rand unavailable")
	}
	return fmt.Sprintf("%06d", n.Int64())
}
