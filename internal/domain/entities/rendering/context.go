// Package rendering provides domain entities for HTML rendering operations
package rendering

import "github.com/AtRiskMedia/tractstack-go/internal/domain/entities/widgets"

// RenderContext provides the context for HTML rendering operations
type RenderContext struct {
	AllNodes         map[string]*NodeRenderData `json:"allNodes,omitempty"`
	ParentNodes      map[string][]string        `json:"parentNodes,omitempty"`
	TenantID         string                     `json:"tenantId,omitempty"`
	SessionID        string                     `json:"sessionId,omitempty"`
	StoryfragmentID  string                     `json:"storyfragmentId,omitempty"`
	ContainingPaneID string                     `json:"containingPaneId,omitempty"`
	WidgetContext    *widgets.WidgetContext     `json:"widgetContext,omitempty"`
}
