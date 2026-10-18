package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

// NewToken returns a URL-safe random token with n random bytes.
func NewToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// NewAccessToken returns a customer access token (32 random bytes).
func NewAccessToken() string { return NewToken(32) }

// TokenDigest returns the hex SHA-256 digest stored instead of a token.
func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewInviteCode returns a registration invite code. Invite codes are stored
// in plain text; they only allow submitting applications for approval.
func NewInviteCode() string { return NewToken(12) }

// EqualConstantTime compares two strings in constant time.
func EqualConstantTime(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
