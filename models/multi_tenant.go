// Package models provides multi-tenant types
package models

import (
	"fmt"
	"regexp"
	"strings"
)

// TenantProvisioningRequest holds the request structure for tenant provisioning
type TenantProvisioningRequest struct {
	TenantID         string `json:"tenantId" binding:"required"`
	AdminPassword    string `json:"adminPassword" binding:"required"`
	Name             string `json:"name" binding:"required"`
	Email            string `json:"email" binding:"required"`
	TursoEnabled     bool   `json:"tursoEnabled"`
	TursoDatabaseURL string `json:"tursoDatabaseURL,omitempty"`
	TursoAuthToken   string `json:"tursoAuthToken,omitempty"`
}

// TenantCapacityResponse holds the response structure for tenant capacity
type TenantCapacityResponse struct {
	CurrentCount    int      `json:"currentCount"`
	MaxTenants      int      `json:"maxTenants"`
	Available       int      `json:"available"`
	ExistingTenants []string `json:"existingTenants"`
}

// ActivationRequest holds the request structure for tenant activation
type ActivationRequest struct {
	Token string `json:"token" binding:"required"`
}

// TenantActivationEmailData holds data for the activation email template
type TenantActivationEmailData struct {
	TenantID      string
	Name          string
	Email         string
	ActivationURL string
	ExpiresIn     string
}

// ValidateTenantID validates tenant ID according to multi-tenant rules
func ValidateTenantID(tenantID string) error {
	// Must be 3-12 characters
	if len(tenantID) < 3 || len(tenantID) > 12 {
		return fmt.Errorf("tenant ID must be 3-12 characters long")
	}

	// Must be lowercase
	if tenantID != strings.ToLower(tenantID) {
		return fmt.Errorf("tenant ID must be lowercase")
	}

	// Only alphanumeric and dashes
	validPattern := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validPattern.MatchString(tenantID) {
		return fmt.Errorf("tenant ID can only contain lowercase letters, numbers, and dashes")
	}

	// Cannot be "default" (reserved)
	if tenantID == "default" {
		return fmt.Errorf("'default' is a reserved tenant ID")
	}

	return nil
}
