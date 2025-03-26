package config

import (
	"reflect"
	"strings"
)

// MergeStructure merges an update structure into a body
// If replace is true, it replaces existing values; otherwise, it adds to them
func MergeStructure(body map[string]any, update map[string]any, replace bool) bool {
	modified := false

	for key, updateValue := range update {
		// If the update value is a map, recurse into it
		if updateMap, ok := updateValue.(map[string]any); ok {
			if actualValue, exists := body[key]; exists {
				if actualMap, ok := actualValue.(map[string]any); ok {
					if MergeStructure(actualMap, updateMap, replace) {
						modified = true
					}
				} else if replace {
					body[key] = updateMap
					modified = true
				}
			} else {
				body[key] = updateMap
				modified = true
			}
		} else if updateArray, ok := updateValue.([]any); ok {
			// Handle arrays
			if actualValue, exists := body[key]; exists {
				if actualArray, ok := actualValue.([]any); ok {
					if replace {
						// For replace, we need to handle array replacements intelligently
						newArray := make([]any, 0)
						replacedIndices := make(map[int]bool)

						// First, identify which elements in the original array should be replaced
						for _, updateItem := range updateArray {
							// Try to find a matching element to replace
							for i, actualItem := range actualArray {
								if !replacedIndices[i] && isReplacementCandidate(actualItem, updateItem) {
									// Mark this index as replaced
									replacedIndices[i] = true
									break
								}
							}

							// Add the update item to the new array
							newArray = append(newArray, updateItem)
						}

						// Add all non-replaced items from the original array
						for i, actualItem := range actualArray {
							if !replacedIndices[i] {
								newArray = append(newArray, actualItem)
							}
						}

						body[key] = newArray
						modified = true
					} else {
						// For upsert, check if we're dealing with key-value pairs
						isKeyValueArray := isKeyValueArray(updateArray)

						if isKeyValueArray {
							// Create a map of keys to their values for the current array
							keyMap := make(map[string]int)
							for i, item := range actualArray {
								if str, ok := item.(string); ok {
									parts := strings.SplitN(str, "=", 2)
									if len(parts) > 0 {
										keyMap[parts[0]] = i
									}
								}
							}

							// Create a copy of the original array
							newArray := make([]any, len(actualArray))
							copy(newArray, actualArray)

							// Process each update item
							arrayModified := false
							for _, updateItem := range updateArray {
								if str, ok := updateItem.(string); ok {
									parts := strings.SplitN(str, "=", 2)
									if len(parts) > 0 {
										// If this key already exists, update it
										if idx, exists := keyMap[parts[0]]; exists {
											newArray[idx] = str
											arrayModified = true
										} else {
											// Otherwise append it
											newArray = append(newArray, str)
											arrayModified = true
										}
									}
								}
							}

							if arrayModified {
								body[key] = newArray
								modified = true
							}
						} else {
							// For regular arrays, just append items not already in the array
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
								modified = true
							}
						}
					}
				} else if replace {
					body[key] = updateArray
					modified = true
				}
			} else {
				body[key] = updateArray
				modified = true
			}
		} else {
			// Handle simple values
			if _, exists := body[key]; !exists || replace {
				body[key] = updateValue
				modified = true
			}
		}
	}

	return modified
}

// DeleteMatchingFields deletes fields that match a structure
func DeleteMatchingFields(body map[string]any, match map[string]any) bool {
	modified := false

	for key, matchValue := range match {
		actualValue, exists := body[key]
		if !exists {
			continue
		}

		// If the match value is a map, recurse into it
		if matchMap, ok := matchValue.(map[string]any); ok {
			if actualMap, ok := actualValue.(map[string]any); ok {
				if DeleteMatchingFields(actualMap, matchMap) {
					modified = true
				}
				// If the map is now empty, delete it
				if len(actualMap) == 0 {
					delete(body, key)
					modified = true
				}
			}
		} else if matchArray, ok := matchValue.([]any); ok {
			// Handle arrays (like Env)
			if actualArray, ok := actualValue.([]any); ok {
				newArray := make([]any, 0, len(actualArray))
				deleted := false

				for _, item := range actualArray {
					shouldDelete := false
					for _, matchItem := range matchArray {
						// Use MatchValue to support regex patterns
						if MatchValue(matchItem, item) {
							shouldDelete = true
							break
						}
					}

					if !shouldDelete {
						newArray = append(newArray, item)
					} else {
						deleted = true
					}
				}

				if deleted {
					if len(newArray) > 0 {
						body[key] = newArray
					} else {
						delete(body, key)
					}
					modified = true
				}
			}
		} else {
			// Handle simple values
			if MatchValue(matchValue, actualValue) {
				delete(body, key)
				modified = true
			}
		}
	}

	return modified
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
