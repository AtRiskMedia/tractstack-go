// Package lisp provides lexer
package lisp

import (
	"strconv"
)

// LispToken represents a token in the parsed lisp expression
type LispToken any

// LispLexer tokenizes a lisp expression, handling quotes, parentheses, and comments
// Returns tokens and remaining string after parsing
func LispLexer(payload string, inString bool) ([]LispToken, string, error) {
	tokens := []LispToken{}
	curToken := ""

	for i := 0; i < len(payload); i++ {
		char := payload[i : i+1]

		if char == `"` && !inString {
			// Start of quoted string - recurse
			tokenized, remaining, err := LispLexer(payload[i+1:], true)
			if err != nil {
				return nil, "", err
			}
			tokens = append(tokens, tokenized)
			payload = remaining
			i = -1
		} else if char == `"` && inString {
			// End of quoted string
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			return tokens, payload[i+1:], nil
		} else if char == "(" {
			// Start of nested expression - recurse
			tokenized, remaining, err := LispLexer(payload[i+1:], false)
			if err != nil {
				return nil, "", err
			}
			tokens = append(tokens, tokenized)
			payload = remaining
			i = -1
		} else if char == ")" {
			// End of expression
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			return tokens, payload[i+1:], nil
		} else if char == ";" {
			// Skip comments until newline
			for i < len(payload) && payload[i:i+1] != "\n" {
				i++
			}
		} else if isWhitespace(char) && !inString {
			// End of current token
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			curToken = ""
		} else {
			// Add character to current token
			curToken += char
		}
	}

	// Handle remaining token at end of string
	if len(curToken) > 0 {
		tokens = append(tokens, parseToken(curToken))
	}

	return tokens, "", nil
}

// parseToken converts string to number if possible, otherwise returns string
func parseToken(token string) LispToken {
	if num, err := strconv.Atoi(token); err == nil {
		return num
	}
	if num, err := strconv.ParseFloat(token, 64); err == nil {
		return num
	}
	return token
}

// isWhitespace checks if character is whitespace
func isWhitespace(char string) bool {
	return char == " " || char == "\n" || char == "\t"
}
