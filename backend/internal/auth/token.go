package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// Tokens are 256 bits from the system CSPRNG, base64url-encoded. Only their
// SHA-256 is stored: SHA-256 (not Argon2) is right here because the input already
// has full entropy, so there is nothing to brute-force, and verification stays
// cheap on every request.
const tokenBytes = 32

// NewToken returns a fresh opaque token for a session or CSRF value.
func NewToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the lookup key stored in auth_sessions.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// TokensEqual compares two secrets without leaking their common prefix length.
func TokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// NewSessionID returns a token plus the hash to persist, so callers never handle
// the plaintext after issuing it.
func NewSessionID() (token string, hash []byte, err error) {
	token, err = NewToken()
	if err != nil {
		return "", nil, err
	}
	return token, HashToken(token), nil
}
