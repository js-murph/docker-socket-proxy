package config

import (
	"reflect"
	"strings"
)

// MergeStructure merges an update structure into a body
func MergeStructure(body map[string]any, update map[string]any, replace bool) bool {
	modified := false

	for key, updateValue := range update {
		if mergeValue(body, key, updateValue, replace) {
			modified = true
		}
	}

	return modified
}

// mergeValue handles merging a single value into the body
func mergeValue(body map[string]any, key string, updateValue any, replace bool) bool {
	switch v := updateValue.(type) {
	case map[string]any:
		return mergeMap(body, key, v, replace)
	case []any:
		return mergeArray(body, key, v, replace)
	default:
		return mergeSimpleValue(body, key, v, replace)
	}
}

// mergeMap handles merging a map value
func mergeMap(body map[string]any, key string, updateMap map[string]any, replace bool) bool {
	if actualValue, exists := body[key]; exists {
		if actualMap, ok := actualValue.(map[string]any); ok {
			return MergeStructure(actualMap, updateMap, replace)
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

// mergeArray handles merging an array value
func mergeArray(body map[string]any, key string, updateArray []any, replace bool) bool {
	if actualValue, exists := body[key]; exists {
		if actualArray, ok := actualValue.([]any); ok {
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

// replaceArray handles replacing elements in an array
func replaceArray(body map[string]any, key string, actualArray, updateArray []any) bool {
	newArray := make([]any, 0)
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

// upsertArray handles upserting elements into an array
func upsertArray(body map[string]any, key string, actualArray, updateArray []any) bool {
	if isKeyValueArray(updateArray) {
		return upsertKeyValueArray(body, key, actualArray, updateArray)
	}
	return upsertSimpleArray(body, key, actualArray, updateArray)
}

// upsertKeyValueArray handles upserting key-value pairs into an array
func upsertKeyValueArray(body map[string]any, key string, actualArray, updateArray []any) bool {
	keyMap := make(map[string]int)
	for i, item := range actualArray {
		if str, ok := item.(string); ok {
			parts := strings.SplitN(str, "=", 2)
			if len(parts) > 0 {
				keyMap[parts[0]] = i
			}
		}
	}

	newArray := make([]any, len(actualArray))
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

// upsertSimpleArray handles upserting simple values into an array
func upsertSimpleArray(body map[string]any, key string, actualArray, updateArray []any) bool {
	newItems := false
	for _, updateItem := range updateArray {
		found := false
		for _, actualItem := range actualArray {
			if reflect.DeepEqual(actualItem, updateItem) {
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

// mergeSimpleValue handles merging a simple value
func mergeSimpleValue(body map[string]any, key string, updateValue any, replace bool) bool {
	if _, exists := body[key]; !exists || replace {
		body[key] = updateValue
		return true
	}
	return false
}

// DeleteMatchingFields deletes fields that match a structure
func DeleteMatchingFields(body map[string]any, match map[string]any) bool {
	modified := false

	for key, matchValue := range match {
		if deleteValue(body, key, matchValue) {
			modified = true
		}
	}

	return modified
}

// deleteValue handles deleting a single value based on its type
func deleteValue(body map[string]any, key string, matchValue any) bool {
	_, exists := body[key]
	if !exists {
		return false
	}

	switch v := matchValue.(type) {
	case map[string]any:
		return deleteMap(body, key, v)
	case []any:
		return deleteArray(body, key, v)
	default:
		return deleteSimpleValue(body, key, v)
	}
}

// deleteMap handles deleting map values
func deleteMap(body map[string]any, key string, matchMap map[string]any) bool {
	actualMap, ok := body[key].(map[string]any)
	if !ok {
		return false
	}

	modified := DeleteMatchingFields(actualMap, matchMap)

	// If the map is now empty, delete it
	if len(actualMap) == 0 {
		delete(body, key)
		return true
	}

	return modified
}

// deleteArray handles deleting array values
func deleteArray(body map[string]any, key string, matchArray []any) bool {
	actualArray, ok := body[key].([]any)
	if !ok {
		return false
	}

	newArray := make([]any, 0, len(actualArray))
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

// shouldDeleteItem checks if an item should be deleted based on match criteria
func shouldDeleteItem(item any, matchArray []any) bool {
	for _, matchItem := range matchArray {
		if MatchValue(matchItem, item) {
			return true
		}
	}
	return false
}

// deleteSimpleValue handles deleting simple values
func deleteSimpleValue(body map[string]any, key string, matchValue any) bool {
	if MatchValue(matchValue, body[key]) {
		delete(body, key)
		return true
	}
	return false
}

// isKeyValueArray checks if an array contains key-value pairs (strings with "=")
func isKeyValueArray(arr []any) bool {
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

// isReplacementCandidate determines if an actual item should be replaced by an update item
func isReplacementCandidate(actual, update any) bool {
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
	if actualMap, ok := actual.(map[string]any); ok {
		if updateMap, ok := update.(map[string]any); ok {
			// Check if they have at least one matching key-value pair
			for key, updateVal := range updateMap {
				if actualVal, exists := actualMap[key]; exists {
					if reflect.DeepEqual(actualVal, updateVal) {
						return true
					}
				}
			}
		}
	}

	// For other types, check for equality
	return reflect.DeepEqual(actual, update)
}
