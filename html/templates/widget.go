// Package templates provides Widget.astro rendering functionality
package templates

import (
	"fmt"

	"github.com/AtRiskMedia/tractstack-go/cache"
	templates "github.com/AtRiskMedia/tractstack-go/html/templates/widgets"
	"github.com/AtRiskMedia/tractstack-go/models"
)

// WidgetRenderer handles Widget.astro rendering logic - dispatcher for all widget types
type WidgetRenderer struct {
	ctx *models.RenderContext
}

// NewWidgetRenderer creates a new widget renderer
func NewWidgetRenderer(ctx *models.RenderContext) *WidgetRenderer {
	return &WidgetRenderer{ctx: ctx}
}

// Render implements the Widget.astro rendering logic - exact dispatcher pattern
func (wr *WidgetRenderer) Render(nodeID string, hook *models.CodeHook) string {
	if hook == nil {
		return `<div>widget error: no hook</div>`
	}

	// Get CSS classes for the widget - matches Widget.astro: const classNames = getCtx().getNodeClasses(nodeId, `auto`);
	classNames := wr.getNodeClasses(nodeID)

	// Dispatch to specific widget renderers exactly like Widget.astro switch pattern
	switch hook.Hook {
	case "youtube":
		return wr.renderYouTube(nodeID, classNames, hook)
	case "bunny":
		return wr.renderBunny(nodeID, classNames, hook)
	case "signup":
		return wr.renderSignUp(nodeID, classNames, hook)
	case "belief":
		return templates.RenderBelief(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "identifyAs":
		return templates.RenderIdentifyAs(wr.ctx, classNames, *hook.Value1, *hook.Value2, hook.Value3)
	case "toggle":
		return templates.RenderToggle(wr.ctx, classNames, *hook.Value1, *hook.Value2)
	case "resource":
		return wr.renderResource(nodeID, classNames, hook)
	default:
		return fmt.Sprintf(`<div class="%s">unknown widget: %s</div>`, classNames, hook.Hook)
	}
}

// renderYouTube matches Widget.astro youtube condition exactly
func (wr *WidgetRenderer) renderYouTube(nodeID, classNames string, hook *models.CodeHook) string {
	// Matches: hook === "youtube" && value1 && value2
	if hook.Value1 != nil && hook.Value2 != nil && *hook.Value1 != "" && *hook.Value2 != "" {
		return fmt.Sprintf(`<div class="%s"><div>YouTube Widget: %s - %s</div></div>`,
			classNames, *hook.Value1, *hook.Value2)
	}
	return ""
}

// renderBunny matches Widget.astro bunny condition exactly
func (wr *WidgetRenderer) renderBunny(nodeID, classNames string, hook *models.CodeHook) string {
	// Matches: hook === "bunny" && value1 && value2
	if hook.Value1 != nil && hook.Value2 != nil && *hook.Value1 != "" && *hook.Value2 != "" {
		return fmt.Sprintf(`<div class="%s"><div>Bunny Video Widget: %s - %s</div></div>`,
			classNames, *hook.Value1, *hook.Value2)
	}
	return ""
}

// renderSignUp matches Widget.astro signup condition exactly
func (wr *WidgetRenderer) renderSignUp(nodeID, classNames string, hook *models.CodeHook) string {
	// Matches: hook === "signup" && value1
	if hook.Value1 != nil && *hook.Value1 != "" {
		persona := *hook.Value1
		prompt := "Keep in touch!" // Default value
		if hook.Value2 != nil && *hook.Value2 != "" {
			prompt = *hook.Value2
		}
		clarifyConsent := hook.Value3 == "true"

		return fmt.Sprintf(`<div class="%s"><div>SignUp Widget: %s - %s (consent: %t)</div></div>`,
			classNames, persona, prompt, clarifyConsent)
	}
	return ""
}

// renderResource matches Widget.astro resource condition exactly
func (wr *WidgetRenderer) renderResource(nodeID, classNames string, hook *models.CodeHook) string {
	// Matches: hook === "resource" && value1
	if hook.Value1 != nil && *hook.Value1 != "" {
		value2 := ""
		if hook.Value2 != nil {
			value2 = *hook.Value2
		}
		return fmt.Sprintf(`<div class="%s"><div><strong>Resource Template (not yet implemented):</strong> %s, %s</div></div>`,
			classNames, *hook.Value1, value2)
	}
	return ""
}

// getNodeClasses retrieves CSS classes for widget - placeholder for Stage 3
func (wr *WidgetRenderer) getNodeClasses(nodeID string) string {
	// For Stage 3, return default "auto" - will connect to actual CSS processor in Stage 4
	return "auto"
}

func (wr *WidgetRenderer) getUserBeliefs() map[string][]string {
	if wr.ctx.SessionID == "" || wr.ctx.StoryfragmentID == "" {
		return nil
	}

	sessionContext, exists := cache.GetGlobalManager().GetSessionBeliefContext(
		wr.ctx.TenantID,
		wr.ctx.SessionID,
		wr.ctx.StoryfragmentID,
	)
	if !exists {
		return nil
	}

	return sessionContext.UserBeliefs
}
