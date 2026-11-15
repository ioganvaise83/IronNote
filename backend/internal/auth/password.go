// Package auth holds credential primitives: Argon2id hashing, session tokens,
// password policy and rate limiting.
//
// Keeping these in one package means there is exactly one implementation of each
// security primitive, and a reviewer has one file to read instead of hunting for
// a second password comparison somewhere in a handler.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. t=3 passes, m=64 MiB, p=2 lanes: comfortably above the
// OWASP minimum, and slow enough that offline guessing stays expensive. They are
// encoded into every hash, so raising them later does not invalidate existing
// passwords — old hashes keep verifying with their own parameters.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // KiB
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

var (
	// ErrInvalidHash means the stored value is not an Argon2id PHC string.
	ErrInvalidHash = errors.New("auth: not a valid argon2id hash")
	// ErrIncompatibleVersion guards against a future format we cannot read.
	ErrIncompatibleVersion = errors.New("auth: incompatible argon2 version")
)

// HashPassword returns a PHC-formatted Argon2id hash:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>
//
// The salt is random per call, so hashing the same password twice yields
// different strings. Only this value is ever stored.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against a stored PHC hash in constant time.
//
// It parses the parameters from the hash rather than using the current constants,
// so hashes created under older settings keep working. An unparseable hash is an
// error, not a silent "match": a corrupted row must not authenticate anybody.
func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))
	// Constant-time comparison: a length-dependent early return would leak the
	// key length, and a byte-by-byte one would leak its prefix.
	return subtle.ConstantTimeCompare(computed, key) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return argonParams{}, nil, nil, ErrIncompatibleVersion
	}

	var params argonParams
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	// Bounds-check before allocating: a hostile row must not be able to make the
	// verifier request an absurd amount of memory.
	if mem < 8*1024 || mem > 1024*1024 || t < 1 || t > 16 || p < 1 || p > 16 {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	params = argonParams{memory: mem, time: t, threads: p}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	return params, salt, key, nil
}

// NeedsRehash reports whether a stored hash was made with weaker parameters than
// the current constants, so a successful login can transparently upgrade it.
func NeedsRehash(encoded string) bool {
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return params.memory < argonMemory ||
		params.time < argonTime ||
		params.threads < argonThreads ||
		len(salt) < argonSaltLen ||
		uint32(len(key)) < argonKeyLen
}

// DummyVerify performs a hash comparison against a throwaway value.
//
// Unknown usernames take this path so that a login attempt costs the same whether
// or not the account exists; without it, response time enumerates valid accounts.
func DummyVerify(password string) {
	_, _ = VerifyPassword(password, dummyHash)
}

// dummyHash is a real Argon2id hash of a random value, generated once at start.
var dummyHash = func() string {
	h, err := HashPassword(randomHex(32))
	if err != nil {
		// Hashing only fails if the system entropy source fails, in which case
		// the process cannot authenticate anyone safely anyway.
		panic("auth: cannot initialise dummy hash: " + err.Error())
	}
	return h
}()

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: entropy source failed: " + err.Error())
	}
	return strconv.FormatInt(int64(len(b)), 10) + base64.RawURLEncoding.EncodeToString(b)
}
