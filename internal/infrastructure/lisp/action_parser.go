package lisp

import (
	"fmt"
	"log"
	"strings"
)

// ParsedAction is a structured data object containing all the necessary information
// for a renderer to build an interactive link or button.
type ParsedAction struct {
	IsValid  bool
	RenderAs string // "a" or "button"
	Href     string
	HxVals   map[string]string

	// For Client-Side Actions
	IsClientSideEvent   bool
	ClientSideEventName string
	ClientSidePayload   map[string]string
}

// lexer tokenizes the actionLisp string, respecting double-quoted segments.
func lexer(input string) []string {
	var tokens []string
	var currentToken strings.Builder
	inString := false

	trimmedInput := strings.TrimSpace(input)

	for _, char := range trimmedInput {
		switch char {
		case '"':
			// When a quote is found, toggle the inString state.
			// If a token is being built, flush it first.
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			inString = !inString
		case '(', ')':
			if !inString {
				// Flush any existing token, then ignore the parenthesis for this parser's needs.
				if currentToken.Len() > 0 {
					tokens = append(tokens, currentToken.String())
					currentToken.Reset()
				}
				continue
			}
			currentToken.WriteRune(char)
		case ' ', '\t', '\n', '\r':
			if inString {
				// If inside a string, whitespace is part of the token.
				currentToken.WriteRune(char)
			} else if currentToken.Len() > 0 {
				// If not in a string, whitespace is a delimiter. Flush the token.
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		default:
			// Any other character is part of the current token.
			currentToken.WriteRune(char)
		}
	}

	// After the loop, flush any remaining token.
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	return tokens
}

// Parse takes a raw actionLisp string and returns a structured ParsedAction object.
// It acts as the single source of truth for actionLisp business logic.
func Parse(actionLisp string, paneID string, homeSlug string) *ParsedAction {
	defaultAction := &ParsedAction{
		IsValid:  false,
		RenderAs: "button", // Safe default to a non-functional button
		HxVals:   nil,
	}

	tokens := lexer(actionLisp)
	if len(tokens) == 0 {
		return defaultAction
	}

	command := tokens[0]
	var params []string
	if len(tokens) > 1 {
		params = tokens[1:]
	}

	switch command {
	case "bunnyMoment":
		lastOpen := strings.LastIndex(actionLisp, "(")
		if lastOpen == -1 {
			log.Printf("WARN: bunnyMoment action has no inner parameters: %q", actionLisp)
			return defaultAction
		}

		// We find the *last* ')'.
		lastClose := strings.LastIndex(actionLisp, ")")
		if lastClose == -1 || lastClose < lastOpen {
			log.Printf("WARN: bunnyMoment action is unclosed: %q", actionLisp)
			return defaultAction
		}

		// This gives us the content *between* the last '(' and last ')'.
		// e.g., "252933/3ed9... 0" or "252933/3ed9... 0)" (from double-nesting)
		innerContent := actionLisp[lastOpen+1 : lastClose]

		// Get the parts.
		parts := strings.Fields(innerContent)

		var videoID, time string

		if len(parts) >= 2 {
			// Take the last two elements.
			videoID = parts[len(parts)-2]
			time = parts[len(parts)-1]

			time = strings.TrimRight(time, ")")

		} else {
			log.Printf("WARN: bunnyMoment action has unparseable inner content: %q", innerContent)
			return defaultAction
		}

		return &ParsedAction{
			IsValid:             true,
			RenderAs:            "button",
			IsClientSideEvent:   true,
			ClientSideEventName: "update-video",
			ClientSidePayload: map[string]string{
				"videoId": videoID,
				"time":    time,
			},
		}

	case "goto":
		href := resolveGotoURL(params, homeSlug)
		hxVals := map[string]string{
			"beliefId":     paneID,
			"beliefType":   "Pane",
			"beliefValue":  "CLICKED",
			"beliefObject": actionLisp,
		}
		return &ParsedAction{
			IsValid:  true,
			RenderAs: "a",
			Href:     href,
			HxVals:   hxVals,
		}

	case "declare", "identifyAs":
		hxVals := buildStateChangePayload(command, params, paneID)
		if hxVals == nil {
			log.Printf("WARN: Could not build state change payload for command %q with params %v", command, params)
			return defaultAction
		}
		return &ParsedAction{
			IsValid:  true,
			RenderAs: "button",
			Href:     "",
			HxVals:   hxVals,
		}
	}

	log.Printf("WARN: lisp.Parse encountered unhandled command: %q", command)
	return defaultAction
}

// buildStateChangePayload creates the specific map for a direct state change hx-vals payload.
func buildStateChangePayload(command string, params []string, paneID string) map[string]string {
	if len(params) < 2 {
		return nil
	}
	beliefID := params[0]
	value := params[1]

	if command == "declare" {
		return map[string]string{
			"beliefId":    beliefID,
			"beliefType":  "Belief",
			"beliefValue": value,
			"paneId":      paneID,
		}
	}

	if command == "identifyAs" {
		return map[string]string{
			"beliefId":     beliefID,
			"beliefType":   "Belief",
			"beliefVerb":   "IDENTIFY_AS",
			"beliefObject": value,
			"paneId":       paneID,
		}
	}

	return nil
}

// resolveGotoURL resolves a (goto ...) action's parameters into a final URL string.
func resolveGotoURL(params []string, homeSlug string) string {
	if len(params) == 0 {
		return "#"
	}

	command := params[0]
	var p1, p2 string
	if len(params) > 1 {
		p1 = params[1]
	}
	if len(params) > 2 {
		p2 = params[2]
	}

	switch command {
	case "home":
		return "/"
	case "storyFragment":
		if p1 != "" && p1 != homeSlug {
			return "/" + p1
		}
		return "/"
	case "storyFragmentPane":
		if p1 != "" && p2 != "" {
			if p1 != homeSlug {
				return fmt.Sprintf("/%s#%s", p1, p2)
			}
			return "/#" + p2
		}
	case "url":
		return p1
	case "context":
		if p1 != "" {
			return "/context/" + p1
		}
	default:
		log.Printf("WARN: resolveGotoURL encountered unhandled goto command: %s", command)
	}

	return "#"
}
