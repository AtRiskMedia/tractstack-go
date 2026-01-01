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

	if ast.CSS != "" && (hr.ctx == nil || !hr.ctx.IsEditorPreview) {
		sb.WriteString("<style>")
		sb.WriteString(ast.CSS)
		sb.WriteString("</style>")
	}

	var bodySb strings.Builder
	for _, node := range ast.Tree {
		hr.renderNode(&bodySb, node)
	}

	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "style", "id").Globally()

	if hr.ctx != nil && hr.ctx.IsEditorPreview {
		p.AllowAttrs("data-ast-id", "contenteditable").Globally()
	}

	sanitizedBody := p.Sanitize(bodySb.String())
	sb.WriteString(sanitizedBody)

	return sb.String()
}

func isEditableTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li":
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

	if hr.ctx != nil && hr.ctx.IsEditorPreview && node.ID != "" {
		sb.WriteString(" data-ast-id=\"")
		sb.WriteString(node.ID)
		sb.WriteString("\"")

		if isEditableTag(node.Tag) {
			sb.WriteString(" contenteditable=\"true\"")
		}
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
