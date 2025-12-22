package modifier

import (
	"reflect"
	"strings"
)

// Modifier defines the interface for modifying request bodies
type Modifier interface {
	Modify(body map[string]interface{}) (map[string]interface{}, bool)
}

// UpsertModifier adds or updates fields in the request body
type UpsertModifier struct {
	updates map[string]interface{}
}

// NewUpsertModifier creates a new UpsertModifier
func NewUpsertModifier(updates map[string]interface{}) *UpsertModifier {
	return &UpsertModifier{
		updates: updates,
	}
}

// Modify applies upsert modifications to the body
func (m *UpsertModifier) Modify(body map[string]interface{}) (map[string]interface{}, bool) {
	modified := mergeStructure(body, m.updates, false)
	return body, modified
}

// ReplaceModifier replaces fields in the request body
type ReplaceModifier struct {
	updates map[string]interface{}
}

// NewReplaceModifier creates a new ReplaceModifier
func NewReplaceModifier(updates map[string]interface{}) *ReplaceModifier {
	return &ReplaceModifier{
		updates: updates,
	}
}

// Modify applies replace modifications to the body
func (m *ReplaceModifier) Modify(body map[string]interface{}) (map[string]interface{}, bool) {
	modified := mergeStructure(body, m.updates, true)
	return body, modified
}

// DeleteModifier removes fields from the request body
type DeleteModifier struct {
	fields map[string]interface{}
}

// NewDeleteModifier creates a new DeleteModifier
func NewDeleteModifier(fields map[string]interface{}) *DeleteModifier {
	return &DeleteModifier{
		fields: fields,
	}
}

// Modify applies delete modifications to the body
func (m *DeleteModifier) Modify(body map[string]interface{}) (map[string]interface{}, bool) {
	modified := deleteMatchingFields(body, m.fields)
	return body, modified
}

// CompositeModifier combines multiple modifiers
type CompositeModifier struct {
	modifiers []Modifier
}

// NewCompositeModifier creates a new CompositeModifier
func NewCompositeModifier(modifiers ...Modifier) *CompositeModifier {
	return &CompositeModifier{
		modifiers: modifiers,
	}
}

// Modify applies all modifiers to the body
func (m *CompositeModifier) Modify(body map[string]interface{}) (map[string]interface{}, bool) {
	modified := false
	currentBody := body
	for _, modifier := range m.modifiers {
		var modResult bool
		currentBody, modResult = modifier.Modify(currentBody)
		if modResult {
			modified = true
		}
	}
	return currentBody, modified
}

// Helper functions

func mergeStructure(body map[string]interface{}, updates map[string]interface{}, replace bool) bool {
	modified := false
	for key, updateValue := range updates {
		if mergeValue(body, key, updateValue, replace) {
			modified = true
		}
	}
	return modified
}

func mergeValue(body map[string]interface{}, key string, updateValue interface{}, replace bool) bool {
	switch v := updateValue.(type) {
	case map[string]interface{}:
		return mergeMap(body, key, v, replace)
	case []interface{}:
		return mergeArray(body, key, v, replace)
	default:
		return mergeSimpleValue(body, key, v, replace)
	}
}

func mergeMap(body map[string]interface{}, key string, updateMap map[string]interface{}, replace bool) bool {
	if actualValue, exists := body[key]; exists {
		if actualMap, ok := actualValue.(map[string]interface{}); ok {
			return mergeStructure(actualMap, updateMap, replace)
		}
		if replace {
			body[key] = updateMap
			return true
		}
	} else {
		body[key] = updateMap
		return true
	}
	return false
}

func mergeArray(body map[string]interface{}, key string, updateArray []interface{}, replace bool) bool {
	if actualValue, exists := body[key]; exists {
		if actualArray, ok := actualValue.([]interface{}); ok {
			if replace {
				return replaceArray(body, key, actualArray, updateArray)
			}
			return upsertArray(body, key, actualArray, updateArray)
		}
		if replace {
			body[key] = updateArray
			return true
		}
	} else {
		body[key] = updateArray
		return true
	}
	return false
}

func replaceArray(body map[string]interface{}, key string, actualArray, updateArray []interface{}) bool {
	newArray := make([]interface{}, 0)
	replacedIndices := make(map[int]bool)

	// Replace matching elements
	for _, updateItem := range updateArray {
		for i, actualItem := range actualArray {
			if !replacedIndices[i] && isReplacementCandidate(actualItem, updateItem) {
				replacedIndices[i] = true
				break
			}
		}
		newArray = append(newArray, updateItem)
	}

	// Add non-replaced elements
	for i, actualItem := range actualArray {
		if !replacedIndices[i] {
			newArray = append(newArray, actualItem)
		}
	}

	body[key] = newArray
	return true
}

func upsertArray(body map[string]interface{}, key string, actualArray, updateArray []interface{}) bool {
	if isKeyValueArray(updateArray) {
		return upsertKeyValueArray(body, key, actualArray, updateArray)
	}
	return upsertSimpleArray(body, key, actualArray, updateArray)
}

func upsertKeyValueArray(body map[string]interface{}, key string, actualArray, updateArray []interface{}) bool {
	keyMap := make(map[string]int)
	for i, item := range actualArray {
		if str, ok := item.(string); ok {
			parts := strings.SplitN(str, "=", 2)
			if len(parts) > 0 {
				keyMap[parts[0]] = i
			}
		}
	}

	newArray := make([]interface{}, len(actualArray))
	copy(newArray, actualArray)
	arrayModified := false

	for _, updateItem := range updateArray {
		if str, ok := updateItem.(string); ok {
			parts := strings.SplitN(str, "=", 2)
			if len(parts) > 0 {
				if idx, exists := keyMap[parts[0]]; exists {
					newArray[idx] = str
					arrayModified = true
				} else {
					newArray = append(newArray, str)
					arrayModified = true
				}
			}
		}
	}

	if arrayModified {
		body[key] = newArray
		return true
	}
	return false
}

func upsertSimpleArray(body map[string]interface{}, key string, actualArray, updateArray []interface{}) bool {
	newItems := false
	for _, updateItem := range updateArray {
		found := false
		for _, actualItem := range actualArray {
			if deepEqual(actualItem, updateItem) {
				found = true
				break
			}
		}
		if !found {
			actualArray = append(actualArray, updateItem)
			newItems = true
		}
	}
	if newItems {
		body[key] = actualArray
		return true
	}
	return false
}

func mergeSimpleValue(body map[string]interface{}, key string, updateValue interface{}, replace bool) bool {
	if _, exists := body[key]; !exists {
		body[key] = updateValue
		return true
	}
	// Check if the value is different
	if !deepEqual(body[key], updateValue) {
		body[key] = updateValue
		return true
	}
	return false
}

func deleteMatchingFields(body map[string]interface{}, fields map[string]interface{}) bool {
	modified := false
	for key, matchValue := range fields {
		if deleteValue(body, key, matchValue) {
			modified = true
		}
	}
	return modified
}

func deleteValue(body map[string]interface{}, key string, matchValue interface{}) bool {
	_, exists := body[key]
	if !exists {
		return false
	}

	switch v := matchValue.(type) {
	case map[string]interface{}:
		return deleteMap(body, key, v)
	case []interface{}:
		return deleteArray(body, key, v)
	default:
		return deleteSimpleValue(body, key, v)
	}
}

func deleteMap(body map[string]interface{}, key string, matchMap map[string]interface{}) bool {
	actualMap, ok := body[key].(map[string]interface{})
	if !ok {
		return false
	}

	modified := deleteMatchingFields(actualMap, matchMap)

	// If the map is now empty, delete it
	if len(actualMap) == 0 {
		delete(body, key)
		return true
	}

	return modified
}

func deleteArray(body map[string]interface{}, key string, matchArray []interface{}) bool {
	actualArray, ok := body[key].([]interface{})
	if !ok {
		return false
	}

	newArray := make([]interface{}, 0, len(actualArray))
	deleted := false

	for _, item := range actualArray {
		if shouldDeleteItem(item, matchArray) {
			deleted = true
			continue
		}
		newArray = append(newArray, item)
	}

	if deleted {
		if len(newArray) > 0 {
			body[key] = newArray
		} else {
			delete(body, key)
		}
		return true
	}

	return false
}

func shouldDeleteItem(item interface{}, matchArray []interface{}) bool {
	for _, matchItem := range matchArray {
		if deepEqual(matchItem, item) {
			return true
		}
		// Check for pattern matching if both are strings
		if itemStr, ok := item.(string); ok {
			if matchStr, ok := matchItem.(string); ok {
				if matchPattern(itemStr, matchStr) {
					return true
				}
			}
		}
	}
	return false
}

func deleteSimpleValue(body map[string]interface{}, key string, matchValue interface{}) bool {
	if _, exists := body[key]; exists {
		// Special case: "anyValue" means delete regardless of actual value
		if matchValue == "anyValue" {
			delete(body, key)
			return true
		}
		// Otherwise, only delete if values match
		if deepEqual(matchValue, body[key]) {
			delete(body, key)
			return true
		}
	}
	return false
}

func isKeyValueArray(arr []interface{}) bool {
	if len(arr) == 0 {
		return false
	}

	for _, item := range arr {
		if str, ok := item.(string); ok {
			if !strings.Contains(str, "=") {
				return false
			}
		} else {
			return false
		}
	}

	return true
}

func isReplacementCandidate(actual, update interface{}) bool {
	// For strings, check if they have the same prefix before "="
	if actualStr, ok := actual.(string); ok {
		if updateStr, ok := update.(string); ok {
			actualParts := strings.SplitN(actualStr, "=", 2)
			updateParts := strings.SplitN(updateStr, "=", 2)

			// If both have a key part (before "="), compare those
			if len(actualParts) > 1 && len(updateParts) > 1 {
				return actualParts[0] == updateParts[0]
			}
		}
	}

	// For maps, check if they have the same key structure
	if actualMap, ok := actual.(map[string]interface{}); ok {
		if updateMap, ok := update.(map[string]interface{}); ok {
			// Check if they have at least one matching key-value pair
			for key, updateVal := range updateMap {
				if actualVal, exists := actualMap[key]; exists {
					if deepEqual(actualVal, updateVal) {
						return true
					}
				}
			}
		}
	}

	// For other types, check for equality
	return deepEqual(actual, update)
}

func deepEqual(a, b interface{}) bool {
	// Use reflect.DeepEqual for proper deep equality check
	return reflect.DeepEqual(a, b)
}

// matchPattern matches a string against a pattern (supports simple regex patterns)
func matchPattern(s, pattern string) bool {
	// This is a simplified pattern matcher
	// In a real implementation, you'd use regexp.Compile
	// For now, we'll implement basic patterns

	// Handle patterns with .* wildcards
	if strings.Contains(pattern, ".*") {
		// Convert pattern to a simple glob pattern
		globPattern := strings.ReplaceAll(pattern, ".*", "*")
		return matchGlob(globPattern, s)
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

	// For other patterns, check for exact match
	return s == pattern
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

	// Check that the string ends with the last part
	if !strings.HasSuffix(s, parts[len(parts)-1]) {
		return false
	}

	// For patterns with more than 2 parts, check that all parts exist in order
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
