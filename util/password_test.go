package util

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		password string
		valid    bool
		why      string
	}{
		{"Secret@123", true, "meets every rule"},
		{"Aa!45678", true, "exactly the 8-char minimum"},
		{"password", false, "no capital, no special"},
		{"Password", false, "no special"},
		{"password!", false, "no capital"},
		{"Pass@1", false, "too short"},
		{"PASSWORD123", false, "no special"},
		{"", false, "empty"},
		{"Pässwörd", false, "non-ASCII letters are not specials"},
	}
	for _, tc := range cases {
		err := ValidatePassword(tc.password)
		if tc.valid && err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil (%s)", tc.password, err, tc.why)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidatePassword(%q) = nil, want error (%s)", tc.password, tc.why)
		}
	}
}

// The temporary passwords handed to admin-provisioned accounts must satisfy
// the same policy their own change-password form enforces.
func TestGenerateTemporaryPasswordMeetsPolicy(t *testing.T) {
	for i := 0; i < 2000; i++ {
		pw := GenerateTemporaryPassword()
		if len(pw) != 12 {
			t.Fatalf("GenerateTemporaryPassword() = %q, want 12 characters", pw)
		}
		if err := ValidatePassword(pw); err != nil {
			t.Fatalf("GenerateTemporaryPassword() = %q, fails policy: %v", pw, err)
		}
	}
}
