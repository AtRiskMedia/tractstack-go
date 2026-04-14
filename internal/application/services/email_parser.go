// Package services provides business logic and orchestration for the application.
package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/email/templates"
)

// BaseBlock is used to unmarshal the generic type field of a polymorphic block
// before deciding how to unmarshal the rest of the raw JSON payload.
type BaseBlock struct {
	Type string `json:"type"`
}

// TextBlock represents a standardized paragraph or heading element.
type TextBlock struct {
	Content string `json:"content"`
	Align   string `json:"align"`
	Color   string `json:"color"`
	IsBold  bool   `json:"isBold"`
}

// ButtonBlock represents a CTA linking to external or dynamic relative URLs.
type ButtonBlock struct {
	Label     string `json:"label"`
	URL       string `json:"url"`
	BgColor   string `json:"bgColor"`
	TextColor string `json:"textColor"`
}

// DividerBlock represents a structural line break.
type DividerBlock struct {
	Color string `json:"color"`
}

// ParseEmailBlocks maps a generic array of json.RawMessage JSON objects
// into strict, compiled Go HTML layout strings.
func ParseEmailBlocks(blocks []json.RawMessage, siteURL string) (string, error) {
	var builder strings.Builder

	for _, rawBlock := range blocks {
		var base BaseBlock
		if err := json.Unmarshal(rawBlock, &base); err != nil {
			return "", fmt.Errorf("failed to parse base block type: %w", err)
		}

		switch base.Type {
		case "text":
			var textBlock TextBlock
			if err := json.Unmarshal(rawBlock, &textBlock); err != nil {
				return "", fmt.Errorf("failed to parse text block: %w", err)
			}
			html := templates.GetParagraphWithOptions(templates.ParagraphProps{
				Text:           textBlock.Content,
				AllowBasicHTML: true, // Content should be plain text but we allow safe tags if passed
				Align:          textBlock.Align,
				Color:          textBlock.Color,
				IsBold:         textBlock.IsBold,
			})
			builder.WriteString(html)

		case "button":
			var buttonBlock ButtonBlock
			if err := json.Unmarshal(rawBlock, &buttonBlock); err != nil {
				return "", fmt.Errorf("failed to parse button block: %w", err)
			}

			// Enforce absolute URLs utilizing the tenant's SITE_URL
			finalURL := buttonBlock.URL
			if strings.HasPrefix(finalURL, "/") {
				finalURL = fmt.Sprintf("%s%s", strings.TrimSuffix(siteURL, "/"), finalURL)
			}

			html := templates.GetButton(templates.ButtonProps{
				Text:            buttonBlock.Label,
				URL:             finalURL,
				BackgroundColor: buttonBlock.BgColor,
				TextColor:       buttonBlock.TextColor,
			})
			builder.WriteString(html)

		case "divider":
			var dividerBlock DividerBlock
			if err := json.Unmarshal(rawBlock, &dividerBlock); err != nil {
				return "", fmt.Errorf("failed to parse divider block: %w", err)
			}
			html := templates.GetDivider(dividerBlock.Color)
			builder.WriteString(html)

		default:
			// Unrecognized block types are silently skipped to maintain transactional delivery
			continue
		}
	}

	return builder.String(), nil
}
