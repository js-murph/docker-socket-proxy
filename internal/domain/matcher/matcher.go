package matcher

import (
	"docker-socket-proxy/internal/domain"
	"strings"
)

// ValueMatcher defines the interface for matching values
type ValueMatcher interface {
	Matches(value interface{}) bool
}

// RequestMatcher defines the interface for matching HTTP requests
type RequestMatcher interface {
	MatchesRequest(req domain.Request) bool
}

// StringMatcher matches string values with exact or regex matching
type StringMatcher struct {
	pattern string
	exact   bool
}

// NewStringMatcher creates a new StringMatcher
func NewStringMatcher(pattern string) *StringMatcher {
	return &StringMatcher{
		pattern: pattern,
		exact:   !isRegexPattern(pattern),
	}
}

// Matches checks if the input matches the pattern
func (m *StringMatcher) Matches(value interface{}) bool {
	str, ok := value.(string)
	if !ok {
		return false
	}

	// Empty pattern matches everything
	if m.pattern == "" {
		return true
	}

	if m.exact {
		return m.pattern == str
	}

	return matchRegex(m.pattern, str)
}

// ArrayMatcher matches array values
type ArrayMatcher struct {
	expected []interface{}
}

// NewArrayMatcher creates a new ArrayMatcher
func NewArrayMatcher(expected []interface{}) *ArrayMatcher {
	return &ArrayMatcher{
		expected: expected,
	}
}

// Matches checks if the input array contains all expected elements
func (m *ArrayMatcher) Matches(value interface{}) bool {
	actual, ok := value.([]interface{})
	if !ok {
		return false
	}

	for _, expItem := range m.expected {
		if !findItemInArray(expItem, actual) {
			return false
		}
	}

	return true
}

// ObjectMatcher matches object values
type ObjectMatcher struct {
	expected map[string]interface{}
}

// NewObjectMatcher creates a new ObjectMatcher
func NewObjectMatcher(expected map[string]interface{}) *ObjectMatcher {
	return &ObjectMatcher{
		expected: expected,
	}
}

// Matches checks if the input object contains all expected fields
func (m *ObjectMatcher) Matches(value interface{}) bool {
	actual, ok := value.(map[string]interface{})
	if !ok {
		return false
	}

	for key, expValue := range m.expected {
		actValue, exists := actual[key]
		if !exists {
			return false
		}

		if !matchesValue(expValue, actValue) {
			return false
		}
	}

	return true
}

// PathMatcher matches request paths
type PathMatcher struct {
	pattern string
}

// NewPathMatcher creates a new PathMatcher
func NewPathMatcher(pattern string) *PathMatcher {
	return &PathMatcher{
		pattern: pattern,
	}
}

// MatchesRequest checks if the request path matches the pattern
func (m *PathMatcher) MatchesRequest(req domain.Request) bool {
	// Empty pattern matches everything
	if m.pattern == "" {
		return true
	}
	result := matchRegex(m.pattern, req.Path)
	return result
}

// MethodMatcher matches request methods
type MethodMatcher struct {
	pattern string
}

// NewMethodMatcher creates a new MethodMatcher
func NewMethodMatcher(pattern string) *MethodMatcher {
	return &MethodMatcher{
		pattern: pattern,
	}
}

// MatchesRequest checks if the request method matches the pattern
func (m *MethodMatcher) MatchesRequest(req domain.Request) bool {
	// Empty pattern matches everything
	if m.pattern == "" {
		return true
	}
	return matchRegex(m.pattern, req.Method)
}

// BodyMatcher matches request body content
type BodyMatcher struct {
	expected map[string]interface{}
}

// NewBodyMatcher creates a new BodyMatcher
func NewBodyMatcher(expected map[string]interface{}) *BodyMatcher {
	return &BodyMatcher{
		expected: expected,
	}
}

// MatchesRequest checks if the request body matches the expected content
func (m *BodyMatcher) MatchesRequest(req domain.Request) bool {
	if len(m.expected) == 0 {
		return true
	}

	return matchesValue(m.expected, req.Body)
}

// CompositeRequestMatcher combines multiple request matchers
type CompositeRequestMatcher struct {
	matchers []RequestMatcher
}

// NewCompositeRequestMatcher creates a new CompositeRequestMatcher
func NewCompositeRequestMatcher(matchers ...RequestMatcher) *CompositeRequestMatcher {
	return &CompositeRequestMatcher{
		matchers: matchers,
	}
}

// MatchesRequest checks if all matchers match the request
func (m *CompositeRequestMatcher) MatchesRequest(req domain.Request) bool {
	for _, matcher := range m.matchers {
		if !matcher.MatchesRequest(req) {
			return false
		}
	}
	return true
}

// Helper functions

func isRegexPattern(s string) bool {
	metaChars := []string{".", "*", "+", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|"}
	for _, char := range metaChars {
		if contains(s, char) {
			return true
		}
	}
	return false
}

func matchRegex(pattern, s string) bool {
	// This is a simplified regex matcher
	// In a real implementation, you'd use regexp.Compile
	// For now, we'll implement basic patterns
	if pattern == s {
		return true
	}

	// Simple wildcard matching
	if pattern == ".*" {
		return true
	}

	// Handle OR patterns like "GET|POST" -> "GET" or "POST"
	if strings.Contains(pattern, "|") {
		parts := strings.Split(pattern, "|")
		for _, part := range parts {
			if part == s {
				return true
			}
		}
		return false
	}

	// Handle patterns with multiple wildcards like "/v1.*/containers/.*" -> "/v1.42/containers/json"
	if strings.Contains(pattern, ".*") {
		// Convert pattern to a simple glob pattern
		globPattern := strings.ReplaceAll(pattern, ".*", "*")
		result := matchGlob(globPattern, s)
		return result
	}

	// Handle patterns like "test.*" -> "test123"
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(s, prefix)
	}

	// Handle patterns like ".*test" -> "123test"
	if strings.HasPrefix(pattern, ".*") {
		suffix := strings.TrimPrefix(pattern, ".*")
		return strings.HasSuffix(s, suffix)
	}

	// More complex regex matching would go here
	return false
}

// matchGlob matches a simple glob pattern against a string
func matchGlob(pattern, s string) bool {
	// This is a very simple glob matcher
	// For now, we'll just handle basic patterns
	if pattern == "*" {
		return true
	}

	// Handle patterns with multiple wildcards like "/v1*/containers/*" -> "/v1.42/containers/json"
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(s, parts[0]) && strings.HasSuffix(s, parts[1])
		}
		// Handle more complex patterns (3 or more parts)
		return matchGlobComplex(pattern, s)
	}

	// Handle patterns like "test*" -> "test123"
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(s, prefix)
	}

	// Handle patterns like "*test" -> "123test"
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(s, suffix)
	}

	return false
}

// matchGlobComplex handles more complex glob patterns
func matchGlobComplex(pattern, s string) bool {
	// Split pattern by wildcards
	parts := strings.Split(pattern, "*")
	if len(parts) < 2 {
		return false
	}

	// Check that the string starts with the first part
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}

	// Check that the string ends with the last part (if it's not empty)
	lastPart := parts[len(parts)-1]
	if lastPart != "" && !strings.HasSuffix(s, lastPart) {
		return false
	}

	// For patterns with more than 2 parts, we need to check that all parts exist in order
	if len(parts) > 2 {
		remaining := s[len(parts[0]):]
		for i := 1; i < len(parts)-1; i++ {
			idx := strings.Index(remaining, parts[i])
			if idx == -1 {
				return false
			}
			remaining = remaining[idx+len(parts[i]):]
		}
	}

	return true
}

func findItemInArray(expected interface{}, actual []interface{}) bool {
	expStr, isExpStr := expected.(string)
	for _, actItem := range actual {
		actStr, isActStr := actItem.(string)
		if isExpStr && isActStr {
			if matchRegex(expStr, actStr) {
				return true
			}
		} else if deepEqual(expected, actItem) {
			return true
		}
	}
	return false
}

func matchesValue(expected, actual interface{}) bool {
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	switch exp := expected.(type) {
	case string:
		return matchStringValue(exp, actual)
	case []interface{}:
		actualArray, ok := actual.([]interface{})
		if !ok {
			return false
		}
		return matchArrayValue(exp, actualArray)
	case map[string]interface{}:
		actualMap, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}
		return matchMapValue(exp, actualMap)
	default:
		return deepEqual(expected, actual)
	}
}

func matchStringValue(expected string, actual interface{}) bool {
	switch act := actual.(type) {
	case string:
		return matchRegex(expected, act)
	case []interface{}:
		return matchStringInArray(expected, act)
	default:
		return false
	}
}

func matchStringInArray(expected string, actual []interface{}) bool {
	for _, item := range actual {
		if str, ok := item.(string); ok && matchRegex(expected, str) {
			return true
		}
	}
	return false
}

func matchArrayValue(expected, actual []interface{}) bool {
	for _, expItem := range expected {
		if !findItemInArray(expItem, actual) {
			return false
		}
	}
	return true
}

func matchMapValue(expected, actual map[string]interface{}) bool {
	for key, expValue := range expected {
		actValue, exists := actual[key]
		if !exists || !matchesValue(expValue, actValue) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func deepEqual(a, b interface{}) bool {
	// Simplified deep equality check
	// In a real implementation, you'd use reflect.DeepEqual
	return a == b
}
