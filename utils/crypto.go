// Package utils contains encryption helpers
package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/golang-jwt/jwt/v4"
	"github.com/oklog/ulid/v2"
)

func GenerateULID() string {
	return ulid.Make().String()
}

func Encrypt(data, key string) (string, error) {
	if len(key) == 0 {
		log.Printf("ERROR: Empty key provided to Encrypt")
		return "", errors.New("empty encryption key")
	}

	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		log.Printf("ERROR: Invalid key length %d. Must be 16, 24, or 32 bytes", len(key))
		return "", errors.New("invalid key length")
	}

	block, err := aes.NewCipher([]byte(key))
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

func Decrypt(encrypted, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		log.Printf("ERROR: base64 decode failed: %v", err)
		return "", err
	}

	block, err := aes.NewCipher([]byte(key))
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

func GetProfileFromClaims(claims jwt.MapClaims) *models.Profile {
	if profileData, ok := claims["profile"].(map[string]any); ok {
		return &models.Profile{
			Fingerprint:    claims["fingerprint"].(string),
			LeadID:         claims["leadId"].(string),
			Firstname:      profileData["firstname"].(string),
			Email:          profileData["email"].(string),
			ContactPersona: profileData["contactPersona"].(string),
			ShortBio:       profileData["shortBio"].(string),
		}
	}
	return nil
}

func GenerateProfileToken(profile *models.Profile, jwtSecret, aesKey string) (string, error) {
	sharedULID := GenerateULID()
	encryptedULID, err := Encrypt(sharedULID, aesKey)
	if err != nil {
		log.Printf("ERROR: Failed to encrypt ULID in GenerateProfileToken: %v", err)
		return "", err
	}

	claims := jwt.MapClaims{
		"fingerprint": profile.Fingerprint,
		"leadId":      profile.LeadID,
		"profile": map[string]string{
			"firstname":      profile.Firstname,
			"email":          profile.Email,
			"contactPersona": profile.ContactPersona,
			"shortBio":       profile.ShortBio,
		},
		"encryptedEmail": encryptedULID,
		"encryptedCode":  encryptedULID,
		"iat":            time.Now().UTC().Unix(),
		"exp":            time.Now().UTC().Add(30 * 24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	result, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		log.Printf("ERROR: Failed to sign JWT token: %v", err)
		return "", err
	}

	return result, nil
}

func ValidateJWT(tokenString, jwtSecret string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

func EncryptEmail(email, aesKey string) string {
	sharedULID := GenerateULID()
	encrypted, err := Encrypt(sharedULID, aesKey)
	if err != nil {
		log.Printf("ERROR: EncryptEmail failed: %v", err)
		return ""
	}
	return encrypted
}

func GenerateEncryptedCode(aesKey string) string {
	sharedULID := GenerateULID()
	encrypted, err := Encrypt(sharedULID, aesKey)
	if err != nil {
		log.Printf("ERROR: GenerateEncryptedCode failed: %v", err)
		return ""
	}
	return encrypted
}
