// Package templates provides email template components
package templates

import (
	"bytes"
	"html/template"
	"log"
	"net/url"
	"regexp"
	"strings"
)

type ButtonProps struct {
	Text            string
	URL             string
	BackgroundColor string
	TextColor       string
}

// Template data structure for email button
type buttonTemplateData struct {
	BackgroundColor string
	URL             string
	TextColor       string
	Text            string
}

// ParagraphProps controls how paragraph content is handled
type ParagraphProps struct {
	Text           string
	AllowBasicHTML bool // When true, allows safe HTML tags like <strong>, <em>, <a>
}

type paragraphTemplateData struct {
	Text template.HTML // Only used for pre-sanitized content
}

// Compiled templates for email components
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

	paragraphTemplate = template.Must(template.New("emailParagraph").Parse(`<p style="font-family: Helvetica, sans-serif; font-size: 16px; font-weight: normal; margin: 0; margin-bottom: 16px;">{{.Text}}</p>`))
)

// allowedHTMLTags defines safe HTML tags for email content
var allowedHTMLTags = map[string]bool{
	"strong": true,
	"b":      true,
	"em":     true,
	"i":      true,
	"u":      true,
	"br":     true,
	"a":      true, // Allow links
	"img":    true, // Allow images
	"span":   true, // Allow spans for styling
}

// allowedAttributes defines safe attributes per tag
var allowedAttributes = map[string]map[string]bool{
	"a": {
		"href":   true,
		"title":  true,
		"target": true,
	},
	"img": {
		"src":    true,
		"alt":    true,
		"width":  true,
		"height": true,
		"style":  true, // For email inline styles
	},
	"span": {
		"style": true, // For email inline styles
	},
	"strong": {
		"style": true,
	},
	"em": {
		"style": true,
	},
	"b": {
		"style": true,
	},
	"i": {
		"style": true,
	},
}

func GetButton(props ButtonProps) string {
	// Set defaults exactly as before
	backgroundColor := props.BackgroundColor
	if backgroundColor == "" {
		backgroundColor = "#0867ec"
	}

	textColor := props.TextColor
	if textColor == "" {
		textColor = "#ffffff"
	}

	// Validate and sanitize URL
	sanitizedURL := sanitizeEmailURL(props.URL)
	if sanitizedURL == "" {
		log.Printf("Invalid or unsafe URL in email button: %s", props.URL)
		sanitizedURL = "#" // Fallback to safe anchor
	}

	// Validate and sanitize colors (basic hex color validation)
	backgroundColor = sanitizeColor(backgroundColor)
	textColor = sanitizeColor(textColor)

	// Create template data
	templateData := buttonTemplateData{
		BackgroundColor: backgroundColor,
		URL:             sanitizedURL,
		TextColor:       textColor,
		Text:            props.Text, // Text is automatically escaped by template
	}

	// Execute template
	var buf bytes.Buffer
	if err := buttonTemplate.Execute(&buf, templateData); err != nil {
		log.Printf("Error executing email button template: %v", err)
		return `<div style="color: red;">Button template error</div>`
	}

	return buf.String()
}

// GetParagraph safely renders paragraph content with optional basic HTML support
func GetParagraph(text string) string {
	return GetParagraphWithOptions(ParagraphProps{
		Text:           text,
		AllowBasicHTML: false, // Default to safe, escaped text only
	})
}

// GetParagraphWithHTML allows basic HTML tags in paragraph content (use with caution)
func GetParagraphWithHTML(text string) string {
	return GetParagraphWithOptions(ParagraphProps{
		Text:           text,
		AllowBasicHTML: true,
	})
}

// GetParagraphWithOptions provides fine-grained control over paragraph rendering
func GetParagraphWithOptions(props ParagraphProps) string {
	var processedText template.HTML

	if props.AllowBasicHTML {
		// Sanitize and allow only safe HTML tags
		processedText = template.HTML(sanitizeBasicHTML(props.Text))
	} else {
		// Escape all HTML - secure default behavior
		var buf bytes.Buffer
		textTemplate := template.Must(template.New("escapeText").Parse("{{.}}"))
		if err := textTemplate.Execute(&buf, props.Text); err != nil {
			log.Printf("Error escaping paragraph text: %v", err)
			return `<div style="color: red;">Paragraph escaping error</div>`
		}
		processedText = template.HTML(buf.String())
	}

	templateData := paragraphTemplateData{
		Text: processedText,
	}

	var buf bytes.Buffer
	if err := paragraphTemplate.Execute(&buf, templateData); err != nil {
		log.Printf("Error executing email paragraph template: %v", err)
		return `<div style="color: red;">Paragraph template error</div>`
	}

	return buf.String()
}

// sanitizeBasicHTML allows only safe HTML tags and removes dangerous content
func sanitizeBasicHTML(input string) string {
	// Remove script tags and their content completely
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	input = scriptRegex.ReplaceAllString(input, "")

	// Remove dangerous event handlers (onclick, onload, etc.)
	eventRegex := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*["\'][^"\']*["\']`)
	input = eventRegex.ReplaceAllString(input, "")

	// Remove javascript: URLs
	jsRegex := regexp.MustCompile(`(?i)javascript\s*:`)
	input = jsRegex.ReplaceAllString(input, "")

	// Allow only specific safe tags, remove all others
	// This regex captures opening tags, closing tags, and self-closing tags
	tagRegex := regexp.MustCompile(`<(/?)(\w+)([^>]*)>`)

	input = tagRegex.ReplaceAllStringFunc(input, func(match string) string {
		submatches := tagRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return "" // Remove malformed tags
		}

		isClosing := submatches[1] == "/"
		tagName := strings.ToLower(submatches[2])
		attributes := submatches[3]

		// Check if tag is allowed
		if !allowedHTMLTags[tagName] {
			return "" // Remove disallowed tags
		}

		// For closing tags, just return the tag
		if isClosing {
			return "</" + tagName + ">"
		}

		// For self-closing tags without attributes
		if tagName == "br" && strings.TrimSpace(attributes) == "" {
			return "<br>"
		}

		// Sanitize attributes for opening tags
		safeAttributes := sanitizeAttributes(tagName, attributes)
		if safeAttributes == "" {
			return "<" + tagName + ">"
		}

		return "<" + tagName + safeAttributes + ">"
	})

	return input
}

// sanitizeAttributes filters attributes to only allow safe ones
func sanitizeAttributes(tagName, attributes string) string {
	if attributes == "" {
		return ""
	}

	allowedForTag, exists := allowedAttributes[tagName]
	if !exists {
		return "" // No attributes allowed for this tag
	}

	// Parse attributes using regex
	attrRegex := regexp.MustCompile(`(\w+)\s*=\s*["\']([^"\']*)["\']`)
	matches := attrRegex.FindAllStringSubmatch(attributes, -1)

	var safeAttrs []string

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		attrName := strings.ToLower(match[1])
		attrValue := match[2]

		// Check if attribute is allowed for this tag
		if !allowedForTag[attrName] {
			continue
		}

		// Additional validation based on attribute type
		switch attrName {
		case "href":
			// Validate URLs
			if sanitizedURL := sanitizeEmailURL(attrValue); sanitizedURL != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+sanitizedURL+`"`)
			}
		case "src":
			// Validate image URLs (similar to href)
			if sanitizedURL := sanitizeImageURL(attrValue); sanitizedURL != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+sanitizedURL+`"`)
			}
		case "style":
			// Basic CSS validation
			if safeCSS := sanitizeInlineCSS(attrValue); safeCSS != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+safeCSS+`"`)
			}
		case "alt", "title", "width", "height":
			// Text attributes - escape and validate
			if cleanValue := sanitizeTextAttribute(attrValue); cleanValue != "" {
				safeAttrs = append(safeAttrs, attrName+`="`+cleanValue+`"`)
			}
		case "target":
			// Only allow safe target values
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

// sanitizeImageURL validates URLs for images
func sanitizeImageURL(url string) string {
	// Same validation as email URLs but also allow data: URLs for inline images
	if strings.HasPrefix(url, "data:image/") {
		return url // Allow data URLs for images
	}
	return sanitizeEmailURL(url) // Use existing URL validation
}

// sanitizeInlineCSS removes dangerous CSS properties and validates against safe properties
func sanitizeInlineCSS(css string) string {
	// Remove potentially dangerous CSS
	dangerous := []string{
		"javascript:",
		"expression(",
		"@import",
		"behavior:",
		"-moz-binding",
	}

	cssLower := strings.ToLower(css)
	for _, danger := range dangerous {
		if strings.Contains(cssLower, danger) {
			return "" // Block entirely if dangerous content found
		}
	}

	// Allow common safe CSS properties for email styling
	safeProperties := map[string]bool{
		"color":            true,
		"background-color": true,
		"font-size":        true,
		"font-weight":      true,
		"font-family":      true,
		"text-align":       true,
		"text-decoration":  true,
		"margin":           true,
		"margin-top":       true,
		"margin-bottom":    true,
		"margin-left":      true,
		"margin-right":     true,
		"padding":          true,
		"padding-top":      true,
		"padding-bottom":   true,
		"padding-left":     true,
		"padding-right":    true,
		"border":           true,
		"border-radius":    true,
		"width":            true,
		"height":           true,
		"display":          true,
		"line-height":      true,
	}

	// Parse CSS properties
	properties := strings.Split(css, ";")
	var safeProps []string

	for _, prop := range properties {
		prop = strings.TrimSpace(prop)
		if prop == "" {
			continue
		}

		// Split property:value
		parts := strings.SplitN(prop, ":", 2)
		if len(parts) != 2 {
			continue
		}

		propName := strings.TrimSpace(strings.ToLower(parts[0]))
		propValue := strings.TrimSpace(parts[1])

		// Check if property is allowed
		if safeProperties[propName] {
			// Basic value validation - no HTML or javascript
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

// sanitizeTextAttribute cleans text attributes
func sanitizeTextAttribute(text string) string {
	// Remove any HTML/script content from text attributes
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&#39;")

	// Remove javascript: protocols
	if strings.Contains(strings.ToLower(text), "javascript:") {
		return ""
	}

	return text
}

// sanitizeEmailURL validates and sanitizes URLs for email use
func sanitizeEmailURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Parse and validate the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("Invalid email URL: %s, error: %v", rawURL, err)
		return ""
	}

	// Only allow http, https, and mailto schemes for email buttons
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "mailto" {
		log.Printf("Blocked unsafe URL scheme in email: %s", scheme)
		return ""
	}

	// Return the sanitized URL
	return parsedURL.String()
}

// sanitizeColor validates and sanitizes hex color values
func sanitizeColor(color string) string {
	if color == "" {
		return "#000000" // Default to black
	}

	// Remove any whitespace
	color = strings.TrimSpace(color)

	// Must start with # and be followed by 3 or 6 hex digits
	if !strings.HasPrefix(color, "#") {
		return "#000000"
	}

	hex := color[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return "#000000"
	}

	// Check if all characters are valid hex digits
	for _, char := range hex {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return "#000000"
		}
	}

	return color
}
