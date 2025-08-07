// Package security provides secure random generation utilities
package security

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/oklog/ulid/v2"
)

// GenerateULID generates a new ULID string.
func GenerateULID() string {
	return ulid.Make().String()
}

// GenerateSecureToken generates a cryptographically secure random token suitable for URLs.
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateSecureKey creates a cryptographically secure random key and returns it as a hex string.
// This is ideal for generating JWT and AES secrets.
func GenerateSecureKey(length int) (string, error) {
	bytes := make([]byte, length/2) // Each byte becomes two hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure key: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
