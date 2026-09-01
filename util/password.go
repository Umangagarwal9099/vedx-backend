package util

import (
	"crypto/rand"
	"errors"
	"strings"
	"unicode/utf8"
)

// PasswordMinLength is the shortest password the platform accepts.
const PasswordMinLength = 8

// passwordSpecialChars is the exact set that counts as a "special character".
// Kept as an explicit ASCII list (rather than "anything non-alphanumeric") so
// the rule can be mirrored character-for-character by the admin and student
// portals' client-side validators and stated verbatim in the error message.
const passwordSpecialChars = "!@#$%^&*()-_=+[]{};:'\",.<>/?\\|`~"

// PasswordPolicyMessage is the single human-readable statement of the policy —
// reused by every endpoint that sets a password so the wording a user sees is
// identical whether they registered, changed, or reset.
const PasswordPolicyMessage = "password must be at least 8 characters and include at least one capital letter and one special character (" + passwordSpecialChars + ")"

// ValidatePassword enforces the platform password policy: at least 8
// characters, at least one A–Z capital, and at least one character from
// passwordSpecialChars. Returns nil when the password is acceptable.
func ValidatePassword(password string) error {
	var hasUpper, hasSpecial bool
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case strings.ContainsRune(passwordSpecialChars, r):
			hasSpecial = true
		}
	}
	if utf8.RuneCountInString(password) < PasswordMinLength || !hasUpper || !hasSpecial {
		return errors.New(PasswordPolicyMessage)
	}
	return nil
}

// GenerateTemporaryPassword returns a 12-character random password drawn from
// an unambiguous character set (no 0/O/1/l/I) for staff/student accounts
// created by an admin — the plaintext is shown once and emailed to the user.
// The result always satisfies ValidatePassword.
func GenerateTemporaryPassword() string {
	const charset = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%"
	const upperset = "ABCDEFGHJKMNPQRSTUVWXYZ"
	const specialset = "!@#$%"

	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("password: crypto/rand unavailable")
	}
	for i, v := range b {
		b[i] = charset[int(v)%len(charset)]
	}

	// A uniform draw from charset can miss the capitals or the specials
	// entirely, which would hand the user a temporary password that their own
	// change-password form then rejects. Plant one of each at two distinct
	// positions so the generated password always passes ValidatePassword.
	pick := make([]byte, 4)
	if _, err := rand.Read(pick); err != nil {
		panic("password: crypto/rand unavailable")
	}
	upperAt := int(pick[0]) % len(b)
	specialAt := (upperAt + 1 + int(pick[1])%(len(b)-1)) % len(b)
	b[upperAt] = upperset[int(pick[2])%len(upperset)]
	b[specialAt] = specialset[int(pick[3])%len(specialset)]

	return string(b)
}
