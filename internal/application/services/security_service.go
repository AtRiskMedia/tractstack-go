package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// SecurityService provides security-related helper functions.
type SecurityService struct{}

// NewSecurityService creates a new SecurityService.
func NewSecurityService() *SecurityService {
	return &SecurityService{}
}

// GenerateSecureToken creates a cryptographically secure random token.
func (s *SecurityService) GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
