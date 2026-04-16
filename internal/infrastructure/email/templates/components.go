// Package templates provides email template components
package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// ButtonProps defines the properties for rendering an email button.
type ButtonProps struct {
	Text            string
	URL             string
	BackgroundColor string
	TextColor       string
	SiteURL         string // Support global URL rewriting
}

type buttonTemplateData struct {
	BackgroundColor string
	URL             string
	TextColor       string
	Text            string
}

// ParagraphProps controls how paragraph content and styling are handled.
type ParagraphProps struct {
	Text           string
	AllowBasicHTML bool
	Align          string
	Color          string
	IsBold         bool
	SiteURL        string // Support global URL rewriting
}

type paragraphTemplateData struct {
	Text   template.HTML
	Align  string
	Color  string
	IsBold bool
}

type dividerTemplateData struct {
	Color string
}

var (
	buttonTemplate = template.Must(template.New("emailButton").Parse(`
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" class="btn btn-primary" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; box-sizing: border-box; width: 100%; min-width: 100%;" width="100%">
      <tbody>
        <tr>
          <td align="left" style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; padding-bottom: 16px;" valign="top">
            <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="border-collapse: separate; mso-table-lspace: 0pt; mso-table-rspace: 0pt; width: auto;">
              <tbody>
                <tr>
                  <td style="font-family: Helvetica, sans-serif; font-size: 16px; vertical-align: top; border-radius: 4px; text-align: center; background-color: {{.BackgroundColor}};" valign="top" align="center" bgcolor="{{.BackgroundColor}}">
                    <a href="{{.URL}}" target="_blank" style="border: solid 2px {{.BackgroundColor}}; border-radius: 4px; box-sizing: border-box; cursor: pointer; display: inline-block; font-size: 16px; font-weight: bold; margin: 0; padding: 12px 24px; text-decoration: none; text-transform: capitalize; background-color: {{.BackgroundColor}}; border-color: {{.BackgroundColor}}; color: {{.TextColor}};">{{.Text}}</a>
                  </td>
                </tr>
              </tbody>
            </table>
          </td>
        </tr>
      </tbody>
    </table>`))

	paragraphTemplate = template.Must(template.New("emailParagraph").Parse(`
    <p style="font-family: Helvetica, sans-serif; font-size: 16px; margin: 0; margin-bottom: 16px; text-align: {{.Align}}; color: {{.Color}}; {{if .IsBold}}font-weight: bold;{{else}}font-weight: normal;{{end}}">
      {{.Text}}
    </p>`))

	dividerTemplate = template.Must(template.New("emailDivider").Parse(`
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="width: 100%; margin-bottom: 24px; margin-top: 8px;">
      <tbody>
        <tr>
          <td style="border-top: 1px solid {{.Color}}; width: 100%;"></td>
        </tr>
      </tbody>
    </table>`))

	emailSanitizer *bluemonday.Policy
)

func init() {
	// Initialize strict bluemonday policy to replace legacy custom regex sanitizers
	emailSanitizer = bluemonday.NewPolicy()

	// Whitelist essential formatting layout tags
	emailSanitizer.AllowElements("strong", "b", "em", "i", "u", "br", "a", "img", "span")

	// Allow core safe attributes
	emailSanitizer.AllowAttrs("href", "title", "target").OnElements("a")
	emailSanitizer.AllowAttrs("src", "alt", "width", "height", "style").OnElements("img")
	emailSanitizer.AllowAttrs("style").OnElements("span", "strong", "em", "b", "i")

	// Enforce strict URL requirements
	emailSanitizer.AllowURLSchemes("http", "https", "mailto")
	emailSanitizer.RequireParseableURLs(true)

	// Explicitly whitelist valid inline CSS properties for layout components
	emailSanitizer.AllowStyles(
		"color", "background-color", "font-size", "font-weight",
		"font-family", "text-align", "text-decoration",
		"margin", "margin-top", "margin-bottom", "margin-left", "margin-right",
		"padding", "padding-top", "padding-bottom", "padding-left", "padding-right",
		"border", "border-radius", "width", "height", "display", "line-height",
	).Globally()
}

// GetButton generates the HTML for a styled email button.
func GetButton(props ButtonProps) string {
	backgroundColor := props.BackgroundColor
	if backgroundColor == "" {
		backgroundColor = "#0867ec"
	}

	textColor := props.TextColor
	if textColor == "" {
		textColor = "#ffffff"
	}

	sanitizedURL := RewriteEmailURL(props.URL, props.SiteURL)
	if sanitizedURL == "" {
		sanitizedURL = "#"
	}

	templateData := buttonTemplateData{
		BackgroundColor: sanitizeColor(backgroundColor),
		URL:             sanitizedURL,
		TextColor:       sanitizeColor(textColor),
		Text:            props.Text,
	}

	var buf bytes.Buffer
	if err := buttonTemplate.Execute(&buf, templateData); err != nil {
		return `<div style="color: red;">Button template error</div>`
	}
	return buf.String()
}

// GetDivider generates a safe HTML table-based divider.
func GetDivider(color string) string {
	if color == "" {
		color = "#E5E7EB"
	}

	templateData := dividerTemplateData{
		Color: sanitizeColor(color),
	}

	var buf bytes.Buffer
	if err := dividerTemplate.Execute(&buf, templateData); err != nil {
		return `<div style="color: red;">Divider template error</div>`
	}
	return buf.String()
}

func GetParagraph(text string) string {
	return GetParagraphWithOptions(ParagraphProps{
		Text:           text,
		AllowBasicHTML: false,
	})
}

func GetParagraphWithHTML(text string) string {
	return GetParagraphWithOptions(ParagraphProps{
		Text:           text,
		AllowBasicHTML: true,
	})
}

// GetParagraphWithOptions provides fine-grained control over paragraph rendering,
// including alignment, color, and font weight.
func GetParagraphWithOptions(props ParagraphProps) string {
	var processedText template.HTML

	if props.AllowBasicHTML {
		// Apply bluemonday sanitization and then perform global URL rewriting on the resulting fragment
		sanitized := emailSanitizer.Sanitize(props.Text)
		processedText = template.HTML(RewriteFragmentURLs(sanitized, props.SiteURL))
	} else {
		var buf bytes.Buffer
		textTemplate := template.Must(template.New("escapeText").Parse("{{.}}"))
		if err := textTemplate.Execute(&buf, props.Text); err != nil {
			return `<div style="color: red;">Paragraph escaping error</div>`
		}
		processedText = template.HTML(buf.String())
	}

	align := props.Align
	if align == "" {
		align = "left"
	}
	if align != "left" && align != "center" && align != "right" {
		align = "left"
	}

	color := props.Color
	if color == "" {
		color = "#333333"
	}

	templateData := paragraphTemplateData{
		Text:   processedText,
		Align:  align,
		Color:  sanitizeColor(color),
		IsBold: props.IsBold,
	}

	var buf bytes.Buffer
	if err := paragraphTemplate.Execute(&buf, templateData); err != nil {
		return `<div style="color: red;">Paragraph template error</div>`
	}

	return buf.String()
}

// RewriteEmailURL transforms relative URLs into absolute URLs using the provided SiteURL.
func RewriteEmailURL(rawURL string, siteURL string) string {
	if rawURL == "" {
		return ""
	}

	// Support uncompiled Go template variables.
	if strings.HasPrefix(rawURL, "{{.") && strings.HasSuffix(rawURL, "}}") {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	// If no scheme is present, rewrite as a root-relative path under the SiteURL.
	if parsedURL.Scheme == "" && siteURL != "" {
		base := strings.TrimSuffix(siteURL, "/")
		path := rawURL
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return base + path
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "mailto" {
		return ""
	}

	return parsedURL.String()
}

// RewriteFragmentURLs applies RewriteEmailURL logic to href and src attributes in a sanitized HTML string.
func RewriteFragmentURLs(html string, siteURL string) string {
	if siteURL == "" {
		return html
	}

	// Target root-relative links standardized by bluemonday (double quotes).
	res := html
	res = strings.ReplaceAll(res, `href="/`, fmt.Sprintf(`href="%s/`, strings.TrimSuffix(siteURL, "/")))
	res = strings.ReplaceAll(res, `src="/`, fmt.Sprintf(`src="%s/`, strings.TrimSuffix(siteURL, "/")))

	return res
}

func sanitizeColor(color string) string {
	if color == "" {
		return "#000000"
	}

	color = strings.TrimSpace(color)

	if !strings.HasPrefix(color, "#") {
		return "#000000"
	}

	hex := color[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return "#000000"
	}

	for _, char := range hex {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return "#000000"
		}
	}

	return color
}
