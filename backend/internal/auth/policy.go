package auth

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Password policy.
//
// Length is the dominant factor in password strength, so the minimum is 12
// characters rather than a symbol-quota checklist (which pushes people towards
// "Password1!"). Composition rules are a light backstop against the worst values
// — a single repeated character, the account name itself, a handful of
// well-known strings — not a substitute for length.
const (
	MinPasswordLength = 12
	// Argon2id hashes the whole input; an unbounded body would let an attacker
	// spend server memory on a megabyte password. 128 is far beyond any real one.
	MaxPasswordLength = 128
)

// commonPasswords is deliberately short: it catches the handful of strings that
// actually appear at the top of every breach list, without pretending to be a
// full dictionary. Anything longer than this belongs in a real HIBP check, which
// requires network access we deliberately do not have.
var commonPasswords = map[string]struct{}{
	"password": {}, "password1": {}, "password123": {}, "qwerty": {}, "qwerty123": {},
	"123456789012": {}, "1234567890123": {}, "letmein": {}, "welcome": {}, "welcome123": {},
	"admin123": {}, "administrator": {}, "iloveyou": {}, "monkey": {}, "dragon": {},
	"abc123456789": {}, "qwertyuiop": {}, "passw0rd": {}, "p@ssw0rd": {}, "changeme": {},
	"change-me-please": {}, "ironnote": {}, "gym": {}, "training": {}, "football": {},
}

// ValidatePassword returns a user-facing reason when a password is unacceptable,
// or nil when it passes. The messages are shown in the UI, so they explain what
// to do rather than just refusing.
func ValidatePassword(password, username string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("Пароль должен быть не короче %d символов", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("Пароль длиннее %d символов", MaxPasswordLength)
	}

	lower := strings.ToLower(password)
	if _, bad := commonPasswords[lower]; bad {
		return fmt.Errorf("Этот пароль слишком распространён — выберите другой")
	}
	if username != "" && strings.Contains(lower, strings.ToLower(username)) {
		return fmt.Errorf("Пароль не должен содержать логин")
	}

	// One character repeated 12+ times ("aaaaaaaaaaaa") passes a length check but
	// has almost no entropy.
	if isSingleRepeatedRune(password) {
		return fmt.Errorf("Пароль слишком простой — используйте разные символы")
	}

	var hasLetter, hasOther bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		default:
			hasOther = true
		}
	}
	if !hasLetter || !hasOther {
		return fmt.Errorf("Пароль должен содержать буквы и хотя бы одну цифру или символ")
	}
	return nil
}

func isSingleRepeatedRune(s string) bool {
	runes := []rune(s)
	if len(runes) < 2 {
		return true
	}
	first := runes[0]
	for _, r := range runes[1:] {
		if r != first {
			return false
		}
	}
	return true
}

// ValidateUsername enforces the shape the database also enforces, so the user
// gets a clear message before the constraint fires.
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("Логин должен быть от 3 до 32 символов")
	}
	for _, r := range username {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("Логин может содержать только латинские буквы, цифры, точку, дефис и подчёркивание")
		}
	}
	return nil
}

// IsWeakBootstrapPassword reports whether a bootstrap password is obviously
// unsafe for a machine reachable from the internet. It is checked at startup so a
// forgotten placeholder cannot become a live public credential.
func IsWeakBootstrapPassword(password string) bool {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return true
	}
	lower := strings.ToLower(password)
	if _, bad := commonPasswords[lower]; bad {
		return true
	}
	for _, placeholder := range []string{"change-me", "changeme", "password", "example", "secret"} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return isSingleRepeatedRune(password)
}
