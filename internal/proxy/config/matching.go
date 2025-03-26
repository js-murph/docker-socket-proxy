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
		// String matching
		switch act := actual.(type) {
		case string:
			// If it looks like a regex, use regex matching
			if isRegexPattern(exp) {
				return matchRegex(exp, act)
			}
			// Otherwise, use exact matching
			return exp == act
		case []interface{}:
			// Check if any array element matches the string
			for _, item := range act {
				if str, ok := item.(string); ok {
					// If it looks like a regex, use regex matching
					if isRegexPattern(exp) {
						if matchRegex(exp, str) {
							return true
						}
					} else {
						// Otherwise, use exact matching
						if exp == str {
							return true
						}
					}
				}
			}
			return false
		default:
			return false
		}
	case []interface{}:
		// Array matching
		switch act := actual.(type) {
		case []interface{}:
			// Check if ALL items in expected are in actual
			for _, expItem := range exp {
				found := false
				expStr, isExpStr := expItem.(string)

				for _, actItem := range act {
					actStr, isActStr := actItem.(string)

					if isExpStr && isActStr {
						// If it looks like a regex, use regex matching
						if isRegexPattern(expStr) {
							if matchRegex(expStr, actStr) {
								found = true
								break
							}
						} else {
							// Otherwise, use exact matching
							if expStr == actStr {
								found = true
								break
							}
						}
					} else if reflect.DeepEqual(expItem, actItem) {
						found = true
						break
					}
				}

				if !found {
					return false // If any expected item is not found, return false
				}
			}
			return true // All expected items were found
		default:
			return false
		}
	case map[string]interface{}:
		// Map matching
		actMap, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}

		// Check if ALL keys in expected are in actual with matching values
		for key, expValue := range exp {
			actValue, exists := actMap[key]
			if !exists || !MatchValue(expValue, actValue) {
				return false
			}
		}
		return true
	default:
		// For other types, use direct equality
		return reflect.DeepEqual(expected, actual)
	}
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
		var body map[string]interface{}
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
func MatchesStructure(body map[string]interface{}, match map[string]interface{}) bool {
	for key, expectedValue := range match {
		actualValue, exists := body[key]
		if !exists {
			return false
		}

		// If the expected value is a map, recurse into it
		if expectedMap, ok := expectedValue.(map[string]interface{}); ok {
			if actualMap, ok := actualValue.(map[string]interface{}); ok {
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
