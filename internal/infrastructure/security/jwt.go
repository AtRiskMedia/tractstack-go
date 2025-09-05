// Package security provides JWT token utilities
package security

import (
	"errors"
	"log"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/golang-jwt/jwt/v4"
)

// ValidateJWT validates a JWT token and returns the claims
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

// GetProfileFromClaims extracts a profile from JWT claims
func GetProfileFromClaims(claims jwt.MapClaims) *user.Profile {
	if profileData, ok := claims["profile"].(map[string]any); ok {
		return &user.Profile{
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

// GenerateProfileToken creates a JWT token for a user profile
func GenerateProfileToken(profile *user.Profile, jwtSecret, aesKey string) (string, error) {
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
