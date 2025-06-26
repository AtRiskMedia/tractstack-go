// Package utils contains encryption helpers
package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/AtRiskMedia/tractstack-go/models"
	"github.com/golang-jwt/jwt/v4"
	"github.com/oklog/ulid/v2"
)

func GenerateULID() string {
	return ulid.Make().String()
}

func Encrypt(data, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(encrypted, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("invalid ciphertext")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
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
		"iat":            time.Now().Unix(),
		"exp":            time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
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
	encrypted, _ := Encrypt(sharedULID, aesKey)
	return encrypted
}

func GenerateEncryptedCode(aesKey string) string {
	sharedULID := GenerateULID()
	encrypted, _ := Encrypt(sharedULID, aesKey)
	return encrypted
}
