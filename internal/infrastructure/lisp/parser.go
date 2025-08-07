// Package lisp provides parser
package lisp

import (
	"fmt"
	"log"
)

// BrandConfig interface for accessing home slug
type BrandConfig interface {
	GetHomeSlug() string
}

// PreParseAction processes parsed lisp tokens and returns target URL
func PreParseAction(payload []LispToken, slug string, isContext bool, brandConfig BrandConfig) string {
	// Handle the nested structure: (goto (storyFragment gallery))
	if len(payload) == 0 {
		return ""
	}

	// Get the first token which should be an array representing the full expression
	var mainExpression []LispToken
	if tokens, ok := payload[0].([]LispToken); ok {
		mainExpression = tokens
	} else {
		mainExpression = payload
	}

	if len(mainExpression) == 0 {
		return ""
	}

	// First element should be "goto"
	command, ok := mainExpression[0].(string)
	if !ok || command != "goto" {
		log.Printf("LispActionPayload preParse misfire - unknown command: %v", mainExpression[0])
		return ""
	}

	if len(mainExpression) < 2 {
		return ""
	}

	// Second element should be the nested expression like (storyFragment gallery)
	var nestedExpression []LispToken
	if tokens, ok := mainExpression[1].([]LispToken); ok {
		nestedExpression = tokens
	} else {
		// Handle flat structure like (goto storyFragment "gallery")
		parameterOne := getParameterString(mainExpression, 1)
		parameterTwo := getParameterString(mainExpression, 2)
		parameterThree := getParameterString(mainExpression, 3)
		return handleGotoCommand(parameterOne, parameterTwo, parameterThree, slug, isContext, brandConfig)
	}

	if len(nestedExpression) == 0 {
		return ""
	}

	// Extract command and parameters from nested expression
	nestedCommand := getParameterString(nestedExpression, 0)
	parameterOne := getParameterString(nestedExpression, 1)
	parameterTwo := getParameterString(nestedExpression, 2)

	return handleGotoCommand(nestedCommand, parameterOne, parameterTwo, slug, isContext, brandConfig)
}

// handleGotoCommand processes goto commands with their parameters
func handleGotoCommand(command, parameterOne, parameterTwo, slug string, isContext bool, brandConfig BrandConfig) string {
	switch command {
	case "storykeep":
		if parameterOne != "" {
			switch parameterOne {
			case "dashboard":
				return "/storykeep"
			case "settings":
				return "/storykeep/settings"
			case "login":
				return "/storykeep/login?force=true"
			case "logout":
				return "/storykeep/logout"
			default:
				log.Printf("LispActionPayload preParse misfire - unknown storykeep action: %v", parameterOne)
			}
		}
		if !isContext {
			return fmt.Sprintf("/%s/edit", slug)
		}
		return fmt.Sprintf("/context/%s/edit", slug)

	case "home":
		return "/"

	case "concierge":
		if parameterOne != "" {
			return fmt.Sprintf("/concierge/%s", parameterOne)
		}
		return "/concierge"

	case "context":
		if parameterOne != "" {
			return fmt.Sprintf("/context/%s", parameterOne)
		}
		return "/context"

	case "storyFragment":
		if parameterOne != "" {
			homeSlug := ""
			if brandConfig != nil {
				homeSlug = brandConfig.GetHomeSlug()
			}
			if parameterOne != homeSlug {
				return fmt.Sprintf("/%s", parameterOne)
			}
		}
		return "/"

	case "storyFragmentPane":
		if parameterOne != "" && parameterTwo != "" {
			homeSlug := ""
			if brandConfig != nil {
				homeSlug = brandConfig.GetHomeSlug()
			}
			if parameterOne != homeSlug {
				return fmt.Sprintf("/%s#%s", parameterOne, parameterTwo)
			}
			return fmt.Sprintf("/#%s", parameterTwo)
		}
		log.Printf("LispActionPayload preParse misfire on goto storyFragmentPane: %v, %v", parameterOne, parameterTwo)

	case "bunny":
		if parameterOne != "" && parameterTwo != "" {
			return fmt.Sprintf("/%s?t=%ss", parameterOne, parameterTwo)
		}
		return ""

	case "url":
		if parameterOne != "" {
			return parameterOne
		}

	case "sandbox":
		if parameterOne != "" {
			switch parameterOne {
			case "claim":
				return "/sandbox/claim"
			}
		}
		return ""

	default:
		log.Printf("LispActionPayload preParse misfire on goto: %v", command)
	}

	return ""
}

// getParameterString safely gets a parameter by index as string
func getParameterString(tokens []LispToken, index int) string {
	if index >= len(tokens) {
		return ""
	}

	if param, ok := tokens[index].(string); ok {
		return param
	}

	return ""
}
