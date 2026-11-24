package handlers

import (
	"net/http"

	"github.com/AtRiskMedia/tractstack-go/internal/application/services"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/performance"
	"github.com/AtRiskMedia/tractstack-go/internal/presentation/http/middleware"
	"github.com/gin-gonic/gin"
)

// AdminAccountHandlers serves /api/v1/admins.
type AdminAccountHandlers struct {
	svc         *services.AdminAccountService
	logger      *logging.ChanneledLogger
	perfTracker *performance.Tracker
}

// NewAdminAccountHandlers creates handlers.
func NewAdminAccountHandlers(svc *services.AdminAccountService, logger *logging.ChanneledLogger, perfTracker *performance.Tracker) *AdminAccountHandlers {
	return &AdminAccountHandlers{svc: svc, logger: logger, perfTracker: perfTracker}
}

func (h *AdminAccountHandlers) List(c *gin.Context) {
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	p := getPrincipal(c)
	list, err := h.svc.List(p, tenantCtx)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, a := range list {
		item := gin.H{
			"id":                 a.ID,
			"username":           a.Username,
			"email":              a.Email,
			"displayName":        a.DisplayName,
			"role":               a.Role,
			"isMain":             a.IsMain,
			"mustChangePassword": a.MustChangePassword,
			"createdAt":          a.CreatedAt,
		}
		if a.LastLoginAt != nil {
			item["lastLoginAt"] = a.LastLoginAt
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"admins": out})
}

func (h *AdminAccountHandlers) Create(c *gin.Context) {
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	var body struct {
		Username           string `json:"username" binding:"required"`
		Email              string `json:"email" binding:"required"`
		DisplayName        string `json:"displayName"`
		Role               string `json:"role" binding:"required"`
		Password           string `json:"password" binding:"required"`
		MustChangePassword bool   `json:"mustChangePassword"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	admin, err := h.svc.Create(getPrincipal(c), services.CreateAdminInput{
		Username:           body.Username,
		Email:              body.Email,
		DisplayName:        body.DisplayName,
		Role:               body.Role,
		Password:           body.Password,
		MustChangePassword: body.MustChangePassword,
	}, tenantCtx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":                 admin.ID,
		"username":           admin.Username,
		"email":              admin.Email,
		"displayName":        admin.DisplayName,
		"role":               admin.Role,
		"isMain":             admin.IsMain,
		"mustChangePassword": admin.MustChangePassword,
	})
}

func (h *AdminAccountHandlers) Update(c *gin.Context) {
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	id := c.Param("id")
	var body struct {
		Username    *string `json:"username"`
		Email       *string `json:"email"`
		DisplayName *string `json:"displayName"`
		Role        *string `json:"role"`
		Password    *string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	admin, err := h.svc.Update(getPrincipal(c), id, services.UpdateAdminInput{
		Username:    body.Username,
		Email:       body.Email,
		DisplayName: body.DisplayName,
		Role:        body.Role,
		Password:    body.Password,
	}, tenantCtx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                 admin.ID,
		"username":           admin.Username,
		"email":              admin.Email,
		"displayName":        admin.DisplayName,
		"role":               admin.Role,
		"isMain":             admin.IsMain,
		"mustChangePassword": admin.MustChangePassword,
	})
}

func (h *AdminAccountHandlers) Delete(c *gin.Context) {
	tenantCtx, ok := middleware.GetTenantContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tenant context not found"})
		return
	}
	if err := h.svc.Delete(getPrincipal(c), c.Param("id"), tenantCtx); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
