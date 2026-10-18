package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/sync/semaphore"
)

// OWASP's Argon2id profile: 19 MiB, two passes, one lane.
// This pre-release accepts only this profile; older databases must be rebuilt.
const (
	argonTime           = 2
	argonMemory         = 19 * 1024
	argonThreads        = 1
	argonKeyLen         = 32
	saltLen             = 16
	passwordConcurrency = 2
)

// MinPasswordLength is the minimum accepted password length in runes.
const MinPasswordLength = 8

var (
	passwordSlots      = semaphore.NewWeighted(passwordConcurrency)
	passwordHashPrefix = fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$",
		argon2.Version, argonMemory, argonTime, argonThreads)
	errInvalidPasswordHash = errors.New("invalid or unsupported password hash")
)

// Precomputed from "dummy password for timing" with the current profile.
// Unknown accounts do the same verification work without hashing at startup.
const dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$cmVnaXN0ZXItbWFuYWdlcg$U988rzEgzxtJ9GK/Ckol7wd9D/fcOxKArop2JICc8ug"

// derivePasswordKey bounds the working memory of all password operations.
// Cancellation stops waiting; Argon2 itself runs to completion once started.
func derivePasswordKey(ctx context.Context, password string, salt []byte) ([]byte, error) {
	if err := passwordSlots.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer passwordSlots.Release(1)
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen), nil
}

// HashPassword returns an encoded Argon2id hash with a fresh random salt.
func HashPassword(ctx context.Context, password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := derivePasswordKey(ctx, password, salt)
	if err != nil {
		return "", err
	}
	return passwordHashPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

// VerifyPassword accepts only the current profile and fixed salt/key lengths.
// Malformed hashes are rejected before acquiring a slot or allocating Argon2 memory.
func VerifyPassword(ctx context.Context, encoded, password string) (bool, error) {
	payload, ok := strings.CutPrefix(encoded, passwordHashPrefix)
	if !ok {
		return false, errInvalidPasswordHash
	}
	saltText, keyText, ok := strings.Cut(payload, "$")
	if !ok {
		return false, errInvalidPasswordHash
	}
	salt, err := decodePasswordPart(saltText, saltLen)
	if err != nil {
		return false, err
	}
	want, err := decodePasswordPart(keyText, argonKeyLen)
	if err != nil {
		return false, err
	}
	got, err := derivePasswordKey(ctx, password, salt)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func decodePasswordPart(text string, size int) ([]byte, error) {
	if len(text) != base64.RawStdEncoding.EncodedLen(size) {
		return nil, errInvalidPasswordHash
	}
	b, err := base64.RawStdEncoding.Strict().DecodeString(text)
	if err != nil || len(b) != size {
		return nil, errInvalidPasswordHash
	}
	return b, nil
}

// VerifyDummy performs the same work as verifying an existing account.
func VerifyDummy(ctx context.Context, password string) error {
	_, err := VerifyPassword(ctx, dummyHash, password)
	return err
}
