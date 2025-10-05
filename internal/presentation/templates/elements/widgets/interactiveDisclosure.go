package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/rendering"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/lisp"
)

// --- Data Structures ---

type DisclosurePayload struct {
	Styles      WidgetStyles     `json:"styles"`
	Disclosures []DisclosureItem `json:"disclosures"`
}

type WidgetStyles struct {
	TextColor string `json:"textColor"`
	BgColor   string `json:"bgColor"`
	BgOpacity int    `json:"bgOpacity"`
}

type DisclosureItem struct {
	BeliefValue string `json:"beliefValue"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	ActionLisp  string `json:"actionLisp"`
}

type tmplData struct {
	Href        string
	Class       string
	Style       template.CSS
	HxVals      template.JS
	Icon        string
	Title       string
	Description string
}

// --- Templates ---

var disclosureStyleTmpl = template.Must(template.New("disclosureStyles").Parse(`
<style>
.disclosure-grid-container { display: grid; grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr)); gap: 1.5rem; width: 100%; }
.disclosure-bento-box { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 2rem; border-radius: 0.75rem; box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1); text-align: center; width: 100%; height: 100%; text-decoration: none; border: 1px solid transparent; transition: transform 0.2s, box-shadow 0.2s; cursor: pointer; color: inherit; background-color: #fff; }
.disclosure-bento-box:hover { transform: scale(1.02); box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1); }
.disclosure-bento-box--active { box-shadow: 0 0 0 3px var(--active-ring-color, #3b82f6); cursor: default; }
.disclosure-bento-box--active:hover { transform: none; box-shadow: 0 0 0 3px var(--active-ring-color, #3b82f6); }
.disclosure-bento-box i { font-size: 2.25rem; line-height: 2.5rem; margin-bottom: 0.5rem; }
.disclosure-bento-box span { font-weight: 700; font-size: 1.25rem; line-height: 1.75rem; }
.disclosure-bento-box p { font-size: 0.875rem; line-height: 1.25rem; opacity: 0.8; margin-top: 0.25rem; }
</style>
`))

var disclosureGridTmpl = template.Must(template.New("interactiveDisclosureGrid").Parse(
	`{{define "link"}}<a href="{{.Href}}" class="{{.Class}}" style="{{.Style}}" hx-post="/api/v1/state" hx-trigger="mousedown" hx-swap="none" hx-vals="{{.HxVals}}">{{template "content" .}}</a>{{end}}` +
		`{{define "button"}}<button type="button" class="{{.Class}}" style="{{.Style}}" hx-post="/api/v1/state" hx-trigger="click" hx-swap="none" hx-vals="{{.HxVals}}">{{template "content" .}}</button>{{end}}` +
		`{{define "content"}}<i class="bi bi-{{.Icon}}"></i><span>{{.Title}}</span>{{if .Description}}<p>{{.Description}}</p>{{end}}{{end}}`,
))

func RenderInteractiveDisclosure(ctx *rendering.RenderContext, classNames, beliefSlug, jsonPayload string) string {
	var payload DisclosurePayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		log.Printf("ERROR: Failed to unmarshal InteractiveDisclosure JSON payload: %v", err)
		return "<!-- error: invalid disclosure payload -->"
	}

	userBeliefs := getUserBeliefs(ctx)
	currentUserBeliefValue := ""
	if values, ok := userBeliefs[beliefSlug]; ok && len(values) > 0 {
		currentUserBeliefValue = values[0]
	}

	var allItemsHTML strings.Builder
	for _, disclosure := range payload.Disclosures {
		parsedAction := lisp.Parse(disclosure.ActionLisp, ctx.ContainingPaneID, ctx.HomeSlug)
		if !parsedAction.IsValid {
			continue
		}

		bentoClasses := "disclosure-bento-box"
		if disclosure.BeliefValue == currentUserBeliefValue {
			bentoClasses += " disclosure-bento-box--active"
		}

		hxValsBytes, err := json.Marshal(parsedAction.HxVals)
		if err != nil {
			continue
		}

		data := tmplData{
			Class:       bentoClasses,
			Style:       template.CSS(formatInlineStyle(payload.Styles)),
			HxVals:      template.JS(hxValsBytes),
			Icon:        disclosure.Icon,
			Title:       disclosure.Title,
			Description: disclosure.Description,
		}

		var tmplName string
		if parsedAction.RenderAs == "a" {
			tmplName = "link"
			data.Href = parsedAction.Href
		} else {
			tmplName = "button"
		}

		var itemBuf strings.Builder
		err = disclosureGridTmpl.ExecuteTemplate(&itemBuf, tmplName, data)
		if err != nil {
			continue
		}
		allItemsHTML.WriteString(itemBuf.String())
	}

	var styleBuf bytes.Buffer
	if err := disclosureStyleTmpl.Execute(&styleBuf, nil); err != nil {
		return "<!-- error executing style template -->"
	}

	finalHTML := fmt.Sprintf(
		`%s<div class="%s disclosure-grid-container">%s</div>`,
		styleBuf.String(),
		classNames,
		allItemsHTML.String(),
	)
	return finalHTML
}

func formatInlineStyle(styles WidgetStyles) string {
	var style strings.Builder
	if styles.BgColor != "" {
		opacity := float64(styles.BgOpacity) / 100.0
		var r, g, b int
		n, err := fmt.Sscanf(styles.BgColor, "#%02x%02x%02x", &r, &g, &b)
		if err == nil && n == 3 {
			style.WriteString(fmt.Sprintf("background-color: rgba(%d, %d, %d, %.2f);", r, g, b, opacity))
		} else {
			log.Printf("WARN: Could not parse BgColor hex value: %s", styles.BgColor)
		}
	}
	if styles.TextColor != "" {
		style.WriteString(fmt.Sprintf("color: %s;", styles.TextColor))
		style.WriteString(fmt.Sprintf("--active-ring-color: %s;", styles.TextColor))
	}
	return style.String()
}
