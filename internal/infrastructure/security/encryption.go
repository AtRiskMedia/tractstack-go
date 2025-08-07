// Package security provides AES encryption utilities
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log"
)

// Encrypt encrypts data using AES-GCM with the provided key
func Encrypt(data, key string) (string, error) {
	if len(key) == 0 {
		log.Printf("ERROR: Empty key provided to Encrypt")
		return "", errors.New("empty encryption key")
	}

	// Hex decode the key first if it's a hex string
	var keyBytes []byte
	if len(key) == 32 || len(key) == 48 || len(key) == 64 {
		// Try to hex decode first
		decoded, err := hex.DecodeString(key)
		if err == nil && (len(decoded) == 16 || len(decoded) == 24 || len(decoded) == 32) {
			keyBytes = decoded
		} else {
			// If hex decode fails or results in wrong length, treat as raw bytes
			keyBytes = []byte(key)
		}
	} else {
		keyBytes = []byte(key)
	}

	if len(keyBytes) != 16 && len(keyBytes) != 24 && len(keyBytes) != 32 {
		log.Printf("ERROR: Invalid key length %d. Must be 16, 24, or 32 bytes", len(keyBytes))
		return "", errors.New("invalid key length")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		log.Printf("ERROR: aes.NewCipher failed: %v", err)
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("ERROR: cipher.NewGCM failed: %v", err)
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Printf("ERROR: Failed to generate nonce: %v", err)
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
	result := base64.StdEncoding.EncodeToString(ciphertext)

	return result, nil
}

// Decrypt decrypts data using AES-GCM with the provided key
func Decrypt(encrypted, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		log.Printf("ERROR: base64 decode failed: %v", err)
		return "", err
	}

	// Hex decode the key first if it's a hex string
	var keyBytes []byte
	if len(key) == 32 || len(key) == 48 || len(key) == 64 {
		// Try to hex decode first
		decoded, err := hex.DecodeString(key)
		if err == nil && (len(decoded) == 16 || len(decoded) == 24 || len(decoded) == 32) {
			keyBytes = decoded
		} else {
			// If hex decode fails or results in wrong length, treat as raw bytes
			keyBytes = []byte(key)
		}
	} else {
		keyBytes = []byte(key)
	}

	if len(keyBytes) != 16 && len(keyBytes) != 24 && len(keyBytes) != 32 {
		log.Printf("ERROR: Invalid key length %d. Must be 16, 24, or 32 bytes", len(keyBytes))
		return "", errors.New("invalid key length")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		log.Printf("ERROR: aes.NewCipher failed in Decrypt: %v", err)
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("ERROR: cipher.NewGCM failed in Decrypt: %v", err)
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		log.Printf("ERROR: invalid ciphertext - too short")
		return "", errors.New("invalid ciphertext")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		log.Printf("ERROR: gcm.Open failed: %v", err)
		return "", err
	}

	return string(plaintext), nil
}

// EncryptEmail encrypts an email using a shared ULID and the provided AES key
func EncryptEmail(email, aesKey string) string {
	sharedULID := GenerateULID()
	encrypted, err := Encrypt(sharedULID, aesKey)
	if err != nil {
		log.Printf("ERROR: EncryptEmail failed: %v", err)
		return ""
	}
	return encrypted
}

// GenerateEncryptedCode generates an encrypted code using a shared ULID and the provided AES key
func GenerateEncryptedCode(aesKey string) string {
	sharedULID := GenerateULID()
	encrypted, err := Encrypt(sharedULID, aesKey)
	if err != nil {
		log.Printf("ERROR: GenerateEncryptedCode failed: %v", err)
		return ""
	}
	return encrypted
}
