package server

import (
	"bytes"
	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// checkACLRules checks if a request is allowed by the ACLs
func (h *ProxyHandler) checkACLRules(r *http.Request, socketConfig *config.SocketConfig) (bool, string) {
	log := logging.GetLogger()

	// Handle nil config - allow by default
	if socketConfig == nil {
		log.Info("No socket configuration found, allowing by default")
		return true, ""
	}

	path := r.URL.Path
	method := r.Method

	log.Info("Checking ACL rules", "path", path, "method", method, "num_rules", len(socketConfig.Rules))

	// If there are no rules, allow by default
	if len(socketConfig.Rules) == 0 {
		log.Info("No rules found, allowing by default")
		return true, ""
	}

	// Check each rule in order
	for i, rule := range socketConfig.Rules {
		log.Info("Checking rule", "index", i, "path", rule.Match.Path, "method", rule.Match.Method)

		// Check if the rule matches
		if !h.ruleMatches(r, rule.Match) {
			continue
		}

		// Rule matched, process actions in order
		log.Info("Rule match result", "index", i, "matched", true,
			"path_pattern", rule.Match.Path, "method_pattern", rule.Match.Method)

		for _, action := range rule.Actions {
			switch action.Action {
			case "allow":
				log.Info("Allow action found", "reason", action.Reason)
				return true, action.Reason
			case "deny":
				log.Info("Deny action found", "reason", action.Reason)
				return false, action.Reason
			default:
				// Continue with next action if not allow/deny
				continue
			}
		}
	}

	// If no rule matches, allow by default
	log.Info("No matching rules found, allowing by default")
	return true, ""
}

// ruleMatches checks if a request matches a rule
func (h *ProxyHandler) ruleMatches(r *http.Request, match config.Match) bool {
	log := logging.GetLogger()
	path := r.URL.Path
	method := r.Method

	// Check if the path matches
	pathMatches := true
	if match.Path != "" {
		var err error
		pathMatches, err = regexp.MatchString(match.Path, path)
		if err != nil {
			log.Error("Error matching path pattern", "error", err)
			return false
		}
	}

	if !pathMatches {
		return false
	}

	// Check if the method matches
	methodMatches := true
	if match.Method != "" {
		var err error
		methodMatches, err = regexp.MatchString(match.Method, method)
		if err != nil {
			log.Error("Error matching method pattern", "error", err)
			return false
		}
	}

	if !methodMatches {
		return false
	}

	// Check if the body matches (for POST/PUT requests)
	if len(match.Contains) > 0 && (method == "POST" || method == "PUT") {
		// Read the request body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error("Error reading request body", "error", err)
			return false
		}
		if err := r.Body.Close(); err != nil {
			return false
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse the JSON body
		var bodyJSON map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			log.Error("Error parsing request body", "error", err)
			return false
		}

		// Check if the body matches the contains criteria
		if !config.MatchValue(match.Contains, bodyJSON) {
			return false
		}
	}

	return true
}

// applyRewriteRules applies rewrite rules to a request
func (s *Server) applyRewriteRules(r *http.Request, socketPath string) error {
	s.configMu.RLock()
	socketConfig, ok := s.socketConfigs[socketPath]
	s.configMu.RUnlock()

	if !ok || socketConfig == nil {
		return nil // No config, no rewrites
	}

	// Only apply rewrites to POST requests that might have a body
	if r.Method != "POST" {
		return nil
	}

	// Read the request body
	if r.Body == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	if err := r.Body.Close(); err != nil {
		return fmt.Errorf("failed to close request body: %w", err)
	}

	// Parse the JSON body
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the original body
		return nil                                        // Not a JSON body, skip rewrites
	}

	// Check each rule in order
	modified := false
	for _, rule := range socketConfig.Rules {
		// Check if the rule matches
		if !matchesRule(r, rule.Match) {
			continue
		}

		// Process actions in order
		for _, action := range rule.Actions {
			// If we find an allow/deny action, stop processing
			if action.Action == "allow" || action.Action == "deny" {
				// Restore the original body and return
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				// If body was modified, apply changes before returning
				if modified {
					newBodyBytes, err := json.Marshal(body)
					if err == nil {
						r.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
						r.ContentLength = int64(len(newBodyBytes))
						r.Header.Set("Content-Length", strconv.Itoa(len(newBodyBytes)))
					}
				}

				return nil
			}

			// Apply rewrite actions
			switch action.Action {
			case "replace":
				if matchesStructure(body, action.Contains) {
					if mergeStructure(body, action.Update, true) {
						modified = true
					}
				}
			case "upsert":
				if mergeStructure(body, action.Update, false) {
					modified = true
				}
			case "delete":
				if deleteMatchingFields(body, action.Contains) {
					modified = true
				}
			}
		}
	}

	// If the body was modified, update the request
	if modified {
		// Marshal the modified body back to JSON
		newBodyBytes, err := json.Marshal(body)
		if err != nil {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the original body
			return fmt.Errorf("failed to marshal modified body: %w", err)
		}

		// Update the request body and Content-Length header
		r.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
		r.ContentLength = int64(len(newBodyBytes))
		r.Header.Set("Content-Length", strconv.Itoa(len(newBodyBytes)))
	} else {
		// Restore the original body
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	return nil
}

// matchesRule checks if a request matches a rewrite rule
func matchesRule(r *http.Request, match config.Match) bool {
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
		if err := r.Body.Close(); err != nil {
			return false
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse the JSON body
		var body map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			return false
		}

		// Check if the body matches the contains criteria
		if !config.MatchValue(match.Contains, body) {
			return false
		}
	}

	return true
}

// matchesStructure checks if a body matches a structure
func matchesStructure(body map[string]interface{}, match map[string]interface{}) bool {
	for key, expectedValue := range match {
		actualValue, exists := body[key]
		if !exists {
			return false
		}

		// If the expected value is a map, recurse into it
		if expectedMap, ok := expectedValue.(map[string]interface{}); ok {
			if actualMap, ok := actualValue.(map[string]interface{}); ok {
				if !matchesStructure(actualMap, expectedMap) {
					return false
				}
			} else {
				return false
			}
		} else if !config.MatchValue(actualValue, expectedValue) {
			return false
		}
	}

	return true
}

// mergeStructure merges an update structure into a body
// If replace is true, it replaces existing values; otherwise, it adds to them
func mergeStructure(body map[string]interface{}, update map[string]interface{}, replace bool) bool {
	modified := false

	for key, updateValue := range update {
		// If the update value is a map, recurse into it
		if updateMap, ok := updateValue.(map[string]interface{}); ok {
			if actualValue, exists := body[key]; exists {
				if actualMap, ok := actualValue.(map[string]interface{}); ok {
					if mergeStructure(actualMap, updateMap, replace) {
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
		} else if updateArray, ok := updateValue.([]interface{}); ok {
			// Handle arrays
			if actualValue, exists := body[key]; exists {
				if actualArray, ok := actualValue.([]interface{}); ok {
					if replace {
						// For replace, we need to handle array replacements intelligently
						newArray := make([]interface{}, 0)
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
							newArray := make([]interface{}, len(actualArray))
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

// isKeyValueArray checks if an array contains key-value pairs (strings with "=")
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
	if actualMap, ok := actual.(map[string]interface{}); ok {
		if updateMap, ok := update.(map[string]interface{}); ok {
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

// deleteMatchingFields deletes fields that match a structure
func deleteMatchingFields(body map[string]interface{}, match map[string]interface{}) bool {
	modified := false

	for key, matchValue := range match {
		actualValue, exists := body[key]
		if !exists {
			continue
		}

		// If the match value is a map, recurse into it
		if matchMap, ok := matchValue.(map[string]interface{}); ok {
			if actualMap, ok := actualValue.(map[string]interface{}); ok {
				if deleteMatchingFields(actualMap, matchMap) {
					modified = true
				}
				// If the map is now empty, delete it
				if len(actualMap) == 0 {
					delete(body, key)
					modified = true
				}
			}
		} else if matchArray, ok := matchValue.([]interface{}); ok {
			// Handle arrays (like Env)
			if actualArray, ok := actualValue.([]interface{}); ok {
				newArray := make([]interface{}, 0, len(actualArray))
				deleted := false

				for _, item := range actualArray {
					shouldDelete := false
					for _, matchItem := range matchArray {
						// Use MatchValue to support regex patterns
						if config.MatchValue(matchItem, item) {
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
			if config.MatchValue(matchValue, actualValue) {
				delete(body, key)
				modified = true
			}
		}
	}

	return modified
}
