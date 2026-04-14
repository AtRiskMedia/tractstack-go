// Package templates provides email template components
package templates

import (
	"bytes"
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

// ButtonProps defines the properties for rendering an email button.
type ButtonProps struct {
	Text            string
	URL             string
	BackgroundColor string
	TextColor       string
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
)

var allowedHTMLTags = map[string]bool{
	"strong": true, "b": true, "em": true, "i": true, "u": true,
	"br": true, "a": true, "img": true, "span": true,
}

var allowedAttributes = map[string]map[string]bool{
	"a":      {"href": true, "title": true, "target": true},
	"img":    {"src": true, "alt": true, "width": true, "height": true, "style": true},
	"span":   {"style": true},
	"strong": {"style": true},
	"em":     {"style": true},
	"b":      {"style": true},
	"i":      {"style": true},
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

	sanitizedURL := sanitizeEmailURL(props.URL)
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
		processedText = template.HTML(sanitizeBasicHTML(props.Text))
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

func sanitizeBasicHTML(input string) string {
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	input = scriptRegex.ReplaceAllString(input, "")

	eventRegex := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*["\'][^"\']*["\']`)
	input = eventRegex.ReplaceAllString(input, "")

	jsRegex := regexp.MustCompile(`(?i)javascript\s*:`)
	input = jsRegex.ReplaceAllString(input, "")

	tagRegex := regexp.MustCompile(`<(/?)(\w+)([^>]*)>`)

	input = tagRegex.ReplaceAllStringFunc(input, func(match string) string {
		submatches := tagRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return ""
		}

		isClosing := submatches[1] == "/"
		tagName := strings.ToLower(submatches[2])
		attributes := submatches[3]

		if !allowedHTMLTags[tagName] {
			return ""
		}

		if isClosing {
			return "</" + tagName + ">"
		}

		if tagName == "br" && strings.TrimSpace(attributes) == "" {
			return "<br>"
		}

		safeAttributes := sanitizeAttributes(tagName, attributes)
		if safeAttributes == "" {
			return "<" + tagName + ">"
		}

		return "<" + tagName + safeAttributes + ">"
	})

	return input
}

func sanitizeAttributes(tagName, attributes string) string {
	if attributes == "" {
		return ""
	}

	allowedForTag, exists := allowedAttributes[tagName]
	if !exists {
		return ""
	}

	attrRegex := regexp.MustCompile(`(\w+)\s*=\s*["\']([^"\']*)["\']`)
	matches := attrRegex.FindAllStringSubmatch(attributes, -1)

	var safeAttrs []string

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		attrName := strings.ToLower(match[1])
		attrValue := match[2]

		if !allowedForTag[attrName] {
			continue
		}

		switch attrName {
		case "href":
			if sanitizedURL := sanitizeEmailURL(attrValue); sanitizedURL != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+sanitizedURL+`"`)
			}
		case "src":
			if sanitizedURL := sanitizeImageURL(attrValue); sanitizedURL != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+sanitizedURL+`"`)
			}
		case "style":
			if safeCSS := sanitizeInlineCSS(attrValue); safeCSS != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+safeCSS+`"`)
			}
		case "alt", "title", "width", "height":
			if cleanValue := sanitizeTextAttribute(attrValue); cleanValue != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+cleanValue+`"`)
			}
		case "target":
			if attrValue == "_blank" || attrValue == "_self" {
				safeAttrs = append(safeAttrs, attrName+`="`+attrValue+`"`)
			}
		}
	}

	if len(safeAttrs) == 0 {
		return ""
	}

	return " " + strings.Join(safeAttrs, " ")
}

func sanitizeImageURL(url string) string {
	if strings.HasPrefix(url, "data:image/") {
		return url
	}
	return sanitizeEmailURL(url)
}

func sanitizeInlineCSS(css string) string {
	dangerous := []string{
		"javascript:", "expression(", "@import", "behavior:", "-moz-binding",
	}

	cssLower := strings.ToLower(css)
	for _, danger := range dangerous {
		if strings.Contains(cssLower, danger) {
			return ""
		}
	}

	safeProperties := map[string]bool{
		"color": true, "background-color": true, "font-size": true,
		"font-weight": true, "font-family": true, "text-align": true,
		"text-decoration": true, "margin": true, "margin-top": true,
		"margin-bottom": true, "margin-left": true, "margin-right": true,
		"padding": true, "padding-top": true, "padding-bottom": true,
		"padding-left": true, "padding-right": true, "border": true,
		"border-radius": true, "width": true, "height": true,
		"display": true, "line-height": true,
	}

	properties := strings.Split(css, ";")
	var safeProps []string

	for _, prop := range properties {
		prop = strings.TrimSpace(prop)
		if prop == "" {
			continue
		}

		parts := strings.SplitN(prop, ":", 2)
		if len(parts) != 2 {
			continue
		}

		propName := strings.TrimSpace(strings.ToLower(parts[0]))
		propValue := strings.TrimSpace(parts[1])

		if safeProperties[propName] {
			if !strings.Contains(propValue, "<") && !strings.Contains(propValue, ">") &&
				!strings.Contains(strings.ToLower(propValue), "javascript:") {
				safeProps = append(safeProps, propName+": "+propValue)
			}
		}
	}

	if len(safeProps) == 0 {
		return ""
	}

	return strings.Join(safeProps, "; ")
}

func sanitizeTextAttribute(text string) string {
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&#39;")

	if strings.Contains(strings.ToLower(text), "javascript:") {
		return ""
	}
	return text
}

func sanitizeEmailURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Support uncompiled Go template variables so they can be parsed later.
	if strings.HasPrefix(rawURL, "{{.") && strings.HasSuffix(rawURL, "}}") {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "mailto" {
		return ""
	}

	return parsedURL.String()
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
