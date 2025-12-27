package templates

import (
	"html/template"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/microcosm-cc/bluemonday"
)

type HastRenderer struct {
	ctx *rendering.RenderContext
}

func (hr *HastRenderer) Render(ast *rendering.HTMLAST) string {
	var sb strings.Builder

	// 1. Render CSS (Unsanitized - trusted from compiler)
	// Skip style injection in Editor Mode so the frontend can control viewport-scoped CSS.
	if ast.CSS != "" && (hr.ctx == nil || !hr.ctx.IsEditorPreview) {
		sb.WriteString("<style>")
		sb.WriteString(ast.CSS)
		sb.WriteString("</style>")
	}

	// 2. Render Body content into a temporary buffer
	var bodySb strings.Builder
	for _, node := range ast.Tree {
		hr.renderNode(&bodySb, node)
	}

	// 3. Define Sanitization Policy
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "style", "id").Globally()

	// If in Editor Mode, allow the interactive hooks
	if hr.ctx != nil && hr.ctx.IsEditorPreview {
		p.AllowAttrs("data-ast-id", "contenteditable").Globally()
	}

	// 4. Sanitize and append body
	// Note: We use SanitizeBytes to avoid extra string allocations, but Sanitize works too.
	sanitizedBody := p.Sanitize(bodySb.String())
	sb.WriteString(sanitizedBody)

	return sb.String()
}

func isEditableTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6",
		"p", "li", "span", "strong", "em",
		"a", "button", "img":
		return true
	default:
		return false
	}
}

func (hr *HastRenderer) renderNode(sb *strings.Builder, node rendering.HTMLASTNode) {
	if node.Tag == "text" {
		sb.WriteString(template.HTMLEscapeString(node.Text))
		return
	}

	sb.WriteString("<")
	sb.WriteString(node.Tag)

	for k, v := range node.Attrs {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=\"")
		sb.WriteString(template.HTMLEscapeString(v))
		sb.WriteString("\"")
	}

	// Inject editor hooks if applicable
	if hr.ctx != nil && hr.ctx.IsEditorPreview && node.ID != "" && isEditableTag(node.Tag) {
		sb.WriteString(" data-ast-id=\"")
		sb.WriteString(node.ID)
		sb.WriteString("\" contenteditable=\"true\"")
	}

	sb.WriteString(">")

	for _, child := range node.Children {
		hr.renderNode(sb, child)
	}

	if !isVoidTag(node.Tag) {
		sb.WriteString("</")
		sb.WriteString(node.Tag)
		sb.WriteString(">")
	}
}

func isVoidTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}
