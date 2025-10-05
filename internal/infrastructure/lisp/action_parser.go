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
}

// Parse takes a raw actionLisp string and returns a structured ParsedAction object.
// It acts as the single source of truth for actionLisp business logic.
func Parse(actionLisp string, paneID string, homeSlug string) *ParsedAction {
	defaultAction := &ParsedAction{
		IsValid:  false,
		RenderAs: "button", // Safe default to a non-functional button
		HxVals:   nil,
	}

	trimmed := strings.Trim(actionLisp, "() ")
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return defaultAction
	}

	command := parts[0]
	innerPayload := strings.Trim(trimmed[len(command):], "() ")
	params := strings.Fields(innerPayload)

	switch command {
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
		hxVals := buildStateChangePayload(command, params, paneID) // FIXED: Pass paneID
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
func buildStateChangePayload(command string, params []string, paneID string) map[string]string { // FIXED: Accept paneID
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
			"paneId":      paneID, // FIXED: Add paneId to payload
		}
	}

	if command == "identifyAs" {
		return map[string]string{
			"beliefId":     beliefID,
			"beliefType":   "Belief",
			"beliefVerb":   "IDENTIFY_AS",
			"beliefObject": value,
			"paneId":       paneID, // FIXED: Add paneId to payload
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
