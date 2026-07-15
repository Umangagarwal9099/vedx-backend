package util

import "crypto/rand"

// GenerateTemporaryPassword returns a 12-character random password drawn from
// an unambiguous character set (no 0/O/1/l/I) for staff/student accounts
// created by an admin — the plaintext is shown once and emailed to the user.
func GenerateTemporaryPassword() string {
	const charset = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%"
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("password: crypto/rand unavailable")
	}
	for i, v := range b {
		b[i] = charset[int(v)%len(charset)]
	}
	return string(b)
}
