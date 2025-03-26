package config

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

// MatchValue checks if a value matches an expected value
func MatchValue(expected, actual interface{}) bool {
	// Handle nil values
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	// Handle different types
	switch exp := expected.(type) {
	case string:
		return matchStringValue(exp, actual)
	case []any:
		actualArray, ok := actual.([]any)
		if !ok {
			return false
		}
		return matchArrayValue(exp, actualArray)
	case map[string]any:
		actualMap, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		return matchMapValue(exp, actualMap)
	default:
		return reflect.DeepEqual(expected, actual)
	}
}

// matchStringValue handles string matching against various types
func matchStringValue(expected string, actual interface{}) bool {
	switch act := actual.(type) {
	case string:
		return matchString(expected, act)
	case []any:
		return matchStringInArray(expected, act)
	default:
		return false
	}
}

// matchString matches a string against another string, handling regex patterns
func matchString(expected, actual string) bool {
	// First try exact match
	if expected == actual {
		return true
	}

	// Only try regex if the pattern contains regex metacharacters
	if isRegexPattern(expected) {
		return matchRegex(expected, actual)
	}

	return false
}

// Helper function to check if a string is a regex pattern
func isRegexPattern(s string) bool {
	// These characters are definitely regex metacharacters when not escaped
	metaChars := []string{".", "*", "+", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|"}

	// Simple check: if it contains any metacharacters, treat it as a regex
	for _, char := range metaChars {
		if strings.Contains(s, char) {
			return true
		}
	}

	return false
}

// matchStringInArray checks if a string matches any element in an array
func matchStringInArray(expected string, actual []any) bool {
	for _, item := range actual {
		if str, ok := item.(string); ok && matchString(expected, str) {
			return true
		}
	}
	return false
}

// matchArrayValue handles array matching
func matchArrayValue(expected, actual []any) bool {
	for _, expItem := range expected {
		if !findItemInArray(expItem, actual) {
			return false
		}
	}
	return true
}

// findItemInArray looks for an item in an array
func findItemInArray(expected interface{}, actual []any) bool {
	expStr, isExpStr := expected.(string)
	for _, actItem := range actual {
		actStr, isActStr := actItem.(string)
		if isExpStr && isActStr {
			if matchString(expStr, actStr) {
				return true
			}
		} else if reflect.DeepEqual(expected, actItem) {
			return true
		}
	}
	return false
}

// matchMapValue handles map matching
func matchMapValue(expected, actual map[string]any) bool {
	for key, expValue := range expected {
		actValue, exists := actual[key]
		if !exists || !MatchValue(expValue, actValue) {
			return false
		}
	}
	return true
}

// Helper function to match a regex pattern against a string
func matchRegex(pattern, s string) bool {
	// Try to compile and use the pattern as a regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		// If compilation fails, fall back to exact match
		return pattern == s
	}
	return re.MatchString(s)
}

// MatchesRule checks if a request matches a rewrite rule
func MatchesRule(r *http.Request, match Match) bool {
	// Check path match
	if match.Path != "" {
		pathMatched, err := regexp.MatchString(match.Path, r.URL.Path)
		if err != nil || !pathMatched {
			return false
		}
	}

	// Check method match
	if match.Method != "" {
		methodMatched, err := regexp.MatchString(match.Method, r.Method)
		if err != nil || !methodMatched {
			return false
		}
	}

	// Check contains criteria
	if len(match.Contains) > 0 {
		// Read and restore the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return false
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse the JSON body
		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			return false
		}

		// Check if the body matches the contains criteria
		if !MatchValue(match.Contains, body) {
			return false
		}
	}

	return true
}

// MatchesStructure checks if a body matches a structure
func MatchesStructure(body map[string]any, match map[string]any) bool {
	for key, expectedValue := range match {
		actualValue, exists := body[key]
		if !exists {
			return false
		}

		// If the expected value is a map, recurse into it
		if expectedMap, ok := expectedValue.(map[string]any); ok {
			if actualMap, ok := actualValue.(map[string]any); ok {
				if !MatchesStructure(actualMap, expectedMap) {
					return false
				}
			} else {
				return false
			}
		} else if !MatchValue(actualValue, expectedValue) {
			return false
		}
	}

	return true
}
