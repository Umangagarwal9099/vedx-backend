package util

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Length caps for the free-text fields on the profile-editing endpoints.
// Enforced server-side because the portals' maxLength attributes are only a
// typing convenience — a crafted request bypasses them entirely and would
// otherwise store unbounded text.
const (
	MaxNameLen      = 50
	MaxShortTextLen = 100
	MaxBioLen       = 500
	MaxAddressLen   = 250
	MaxURLLen       = 500
	MaxSkills       = 30
	MaxSkillLen     = 40
)

var (
	// Optional leading +, then 7–15 digits, with spaces/hyphens/parens allowed
	// as separators anywhere in between.
	phoneDigitsRe = regexp.MustCompile(`^\+?[0-9]{7,15}$`)
	phoneStripRe  = regexp.MustCompile(`[\s\-()]`)
	emailRe       = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)
	pincodeRe     = regexp.MustCompile(`^[0-9]{4,10}$`)
	// Letters (any script), spaces, and the punctuation that legitimately
	// appears in names — no digits, no symbols, no angle brackets.
	nameRe = regexp.MustCompile(`^[\p{L}][\p{L}\s.'\-]*$`)
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ValidateText caps a free-text field's length. Empty is always allowed —
// clearing a field is a valid edit on every profile endpoint.
func ValidateText(field, value string, max int) error {
	if utf8.RuneCountInString(strings.TrimSpace(value)) > max {
		return fmt.Errorf("%s must be at most %d characters", field, max)
	}
	return nil
}

// ValidateName accepts a person's name — letters, spaces, apostrophes,
// hyphens and periods only, so digits and markup can't be stored in a field
// that gets rendered on certificates, emails, and the admin tables.
func ValidateName(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	if utf8.RuneCountInString(trimmed) > MaxNameLen {
		return fmt.Errorf("%s must be at most %d characters", field, MaxNameLen)
	}
	if !nameRe.MatchString(trimmed) {
		return fmt.Errorf("%s may only contain letters, spaces, hyphens and apostrophes", field)
	}
	return nil
}

// ValidatePhone accepts an optional phone number: 7–15 digits, optionally
// prefixed with +, with spaces/hyphens/parentheses tolerated as separators.
// Empty passes — phone is optional on every profile form.
func ValidatePhone(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if !phoneDigitsRe.MatchString(phoneStripRe.ReplaceAllString(trimmed, "")) {
		return fmt.Errorf("%s must be a valid phone number (7–15 digits)", field)
	}
	return nil
}

// ValidateEmailAddr accepts an optional email address.
func ValidateEmailAddr(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if utf8.RuneCountInString(trimmed) > MaxShortTextLen || !emailRe.MatchString(trimmed) {
		return fmt.Errorf("%s must be a valid email address", field)
	}
	return nil
}

// ValidatePincode accepts an optional numeric postal code.
func ValidatePincode(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if !pincodeRe.MatchString(trimmed) {
		return fmt.Errorf("%s must be 4–10 digits", field)
	}
	return nil
}

// ValidateOneOf restricts a field to a fixed set of values. Empty passes —
// every enum field on these forms has a "not set" state.
func ValidateOneOf(field, value string, allowed ...string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	for _, a := range allowed {
		if trimmed == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of: %s", field, strings.Join(allowed, ", "))
}

// ValidateDateOfBirth accepts an optional YYYY-MM-DD date that is a real
// calendar date, not in the future, and within a plausible human lifespan.
func ValidateDateOfBirth(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if !dateRe.MatchString(trimmed) {
		return fmt.Errorf("%s must be in YYYY-MM-DD format", field)
	}
	dob, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return fmt.Errorf("%s is not a valid date", field)
	}
	now := time.Now()
	if dob.After(now) {
		return fmt.Errorf("%s cannot be in the future", field)
	}
	if dob.Before(now.AddDate(-120, 0, 0)) {
		return fmt.Errorf("%s is not a plausible date of birth", field)
	}
	return nil
}

// ValidateSkills caps both the number of skill tags and the length of each,
// and rejects blank entries so the stored array stays renderable as chips.
func ValidateSkills(skills []string) error {
	if len(skills) > MaxSkills {
		return fmt.Errorf("skills must be at most %d entries", MaxSkills)
	}
	for _, s := range skills {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return fmt.Errorf("skills may not contain blank entries")
		}
		if utf8.RuneCountInString(trimmed) > MaxSkillLen {
			return fmt.Errorf("each skill must be at most %d characters", MaxSkillLen)
		}
	}
	return nil
}
