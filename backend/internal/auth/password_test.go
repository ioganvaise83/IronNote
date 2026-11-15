package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	const password = "correct horse battery 42"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if strings.Contains(hash, password) {
		t.Fatal("hash contains the plaintext password")
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected PHC format: %s", hash)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword(password+"x", hash); ok {
		t.Fatal("a wrong password verified")
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same password 123")
	b, _ := HashPassword("same password 123")
	if a == b {
		t.Fatal("two hashes of the same password are identical: salt is not random")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	// A corrupted or attacker-supplied hash must never authenticate.
	cases := []string{
		"",
		"plaintext",
		"$argon2id$v=19$m=65536,t=3,p=2$notbase64$alsonot",
		"$argon2i$v=19$m=65536,t=3,p=2$c2FsdHNhbHRzYWx0c2E$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		// Hostile memory cost: must be rejected before allocating.
		"$argon2id$v=19$m=99999999,t=3,p=2$c2FsdHNhbHRzYWx0c2E$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
		"$argon2id$v=16$m=65536,t=3,p=2$c2FsdHNhbHRzYWx0c2E$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5",
	}
	for _, hash := range cases {
		if ok, err := VerifyPassword("anything", hash); ok || err == nil {
			t.Fatalf("malformed hash %q returned ok=%v err=%v", hash, ok, err)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	good := []string{
		"gym-session-2024",
		"CorrectHorse42",
		"тренировка-2024",
	}
	for _, pw := range good {
		if err := ValidatePassword(pw, "admin"); err != nil {
			t.Fatalf("%q should be accepted: %v", pw, err)
		}
	}

	bad := []struct {
		password string
		reason   string
	}{
		{"short1", "length"},
		{"aaaaaaaaaaaaaaaa", "repeated"},
		{"password", "length"},
		{"changeme", "length"},
		{"admin-password-1", "contains username"},
		{"abcdefghijklmnop", "no digits"},
		{strings.Repeat("x", MaxPasswordLength+1) + "1", "too long"},
	}
	for _, tc := range bad {
		if err := ValidatePassword(tc.password, "admin"); err == nil {
			t.Fatalf("%q (%s) should be rejected", tc.password, tc.reason)
		}
	}
}

func TestUsernamePolicy(t *testing.T) {
	for _, ok := range []string{"admin", "anna.k", "brat_1", "User-2024"} {
		if err := ValidateUsername(ok); err != nil {
			t.Fatalf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "ab", "админ", "with space", "semi;colon", strings.Repeat("a", 33)} {
		if err := ValidateUsername(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestWeakBootstrapPasswordDetection(t *testing.T) {
	if !IsWeakBootstrapPassword("change-me-please") {
		t.Fatal("the documented placeholder must be rejected")
	}
	if !IsWeakBootstrapPassword("short") {
		t.Fatal("a short password must be rejected")
	}
	if IsWeakBootstrapPassword("Fair-Squat-2026") {
		t.Fatal("a reasonable password was rejected")
	}
}

func TestTokensAreUniqueAndHashed(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		token, hash, err := NewSessionID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[token] {
			t.Fatal("duplicate token generated")
		}
		seen[token] = true
		if len(hash) != 32 {
			t.Fatalf("hash length = %d, want 32", len(hash))
		}
		if string(hash) == token {
			t.Fatal("stored value equals the bearer token")
		}
	}
}

func TestNeedsRehashDetectsWeakerParameters(t *testing.T) {
	current, _ := HashPassword("some password 1")
	if NeedsRehash(current) {
		t.Fatal("a fresh hash should not need rehashing")
	}
	// A hash produced with lower memory cost must be upgraded on next login.
	weak := "$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5"
	if !NeedsRehash(weak) {
		t.Fatal("a weak hash should need rehashing")
	}
	if !NeedsRehash("garbage") {
		t.Fatal("an unparseable hash should need rehashing")
	}
}
