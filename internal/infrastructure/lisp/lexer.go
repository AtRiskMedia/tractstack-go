// Package lisp provides lexer
package lisp

import (
	"strconv"
)

// Token represents a token in the parsed lisp expression
type Token any

// Lexer tokenizes a lisp expression, handling quotes, parentheses, and comments
// Returns tokens and remaining string after parsing
func Lexer(payload string, inString bool) ([]Token, string, error) {
	tokens := []Token{}
	curToken := ""

	for i := 0; i < len(payload); i++ {
		char := payload[i : i+1]

		switch {
		case char == `"` && !inString:
			// Start of quoted string - recurse
			tokenized, remaining, err := Lexer(payload[i+1:], true)
			if err != nil {
				return nil, "", err
			}
			tokens = append(tokens, tokenized)
			payload = remaining
			i = -1

		case char == `"` && inString:
			// End of quoted string
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			return tokens, payload[i+1:], nil

		case char == "(":
			// Start of nested expression - recurse
			tokenized, remaining, err := Lexer(payload[i+1:], false)
			if err != nil {
				return nil, "", err
			}
			tokens = append(tokens, tokenized)
			payload = remaining
			i = -1

		case char == ")":
			// End of expression
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			return tokens, payload[i+1:], nil

		case char == ";":
			// Skip comments until newline
			for i < len(payload) && payload[i:i+1] != "\n" {
				i++
			}

		case isWhitespace(char) && !inString:
			// End of current token
			if len(curToken) > 0 {
				tokens = append(tokens, parseToken(curToken))
			}
			curToken = ""

		default:
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
func parseToken(token string) Token {
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
