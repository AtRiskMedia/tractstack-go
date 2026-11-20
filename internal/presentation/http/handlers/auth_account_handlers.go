package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetMe returns the current principal's account fields.
func (h *AuthHandlers) GetMe(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	p := getPrincipal(c)
	if p == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	admin, err := tenantCtx.AdminRepo().FindByID(p.Sub)
	if err != nil || admin == nil {
		c.JSON(http.StatusOK, gin.H{
			"sub":                p.Sub,
			"username":           p.Username,
			"email":              p.Email,
			"displayName":        p.Name,
			"role":               p.Role,
			"isMain":             p.IsMain,
			"mustChangePassword": p.MustChangePassword,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sub":                admin.ID,
		"username":           admin.Username,
		"email":              admin.Email,
		"displayName":        admin.DisplayName,
		"role":               admin.Role,
		"isMain":             admin.IsMain,
		"mustChangePassword": admin.MustChangePassword,
	})
}

// PutMe updates the current user's display name / email.
func (h *AuthHandlers) PutMe(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	p := getPrincipal(c)
	if p == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	var body struct {
		DisplayName *string `json:"displayName"`
		Email       *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	svc := services.NewAdminAccountService(h.logger, h.authService)
	admin, err := svc.UpdateMe(p, body.DisplayName, body.Email, tenantCtx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sub":                admin.ID,
		"username":           admin.Username,
		"email":              admin.Email,
		"displayName":        admin.DisplayName,
		"role":               admin.Role,
		"isMain":             admin.IsMain,
		"mustChangePassword": admin.MustChangePassword,
	})
}

// PostChangePassword changes the current user's password and re-sets the cookie.
func (h *AuthHandlers) PostChangePassword(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	p := getPrincipal(c)
	if p == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword" binding:"required"`
		NewPassword     string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	token, err := h.authService.ChangeOwnPassword(p, body.CurrentPassword, body.NewPassword, tenantCtx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cookieName := "admin_auth"
	if p.Role == "editor" {
		cookieName = "editor_auth"
	}
	c.SetCookie(cookieName, token, 86400, "/", cookieDomainFor(tenantCtx), cookieSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"success": true, "token": token})
}

// DeleteMe deletes the caller's own account (non-main only).
func (h *AuthHandlers) DeleteMe(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	p := getPrincipal(c)
	svc := services.NewAdminAccountService(h.logger, h.authService)
	if err := svc.DeleteSelf(p, tenantCtx); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	domain := cookieDomainFor(tenantCtx)
	secure := cookieSecure(c)
	c.SetCookie("admin_auth", "", -1, "/", domain, secure, true)
	c.SetCookie("editor_auth", "", -1, "/", domain, secure, true)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// PostForgotPassword always returns 200.
func (h *AuthHandlers) PostForgotPassword(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	_ = c.ShouldBindJSON(&body)
	if !allowAuthAttempt(tenantCtx.TenantID, c.ClientIP(), body.Email) {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	hasResend := false
	if tenantCtx.Config != nil && tenantCtx.Config.ResendAPIKey != "" &&
		tenantCtx.Config.BrandConfig != nil &&
		tenantCtx.Config.BrandConfig.AdminEmail != "" &&
		tenantCtx.Config.BrandConfig.AdminEmailName != "" {
		hasResend = true
	}
	h.authService.RequestPasswordReset(body.Email, tenantCtx, hasResend)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// PostResetPassword consumes a reset token (no cookie).
func (h *AuthHandlers) PostResetPassword(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	var body struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if !allowAuthAttempt(tenantCtx.TenantID, c.ClientIP(), body.Token) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts"})
		return
	}
	if err := h.authService.ConsumePasswordReset(body.Token, body.NewPassword, tenantCtx); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetPasswordTokenStatus checks a reset token.
func (h *AuthHandlers) GetPasswordTokenStatus(c *gin.Context) {
	tenantCtx, exists := middleware.GetTenantContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false})
		return
	}
	if !allowAuthAttempt(tenantCtx.TenantID, c.ClientIP(), token) {
		c.JSON(http.StatusTooManyRequests, gin.H{"valid": false})
		return
	}
	valid, purpose := h.authService.PasswordTokenStatus(token, tenantCtx)
	c.JSON(http.StatusOK, gin.H{"valid": valid, "purpose": purpose})
}
