package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
)

// ProxyHandler handles proxying requests to the Docker socket
type ProxyHandler struct {
	dockerSocket  string
	socketConfigs map[string]*config.SocketConfig
	configMu      *sync.RWMutex
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(dockerSocket string, configs map[string]*config.SocketConfig, mu *sync.RWMutex) *ProxyHandler {
	return &ProxyHandler{
		dockerSocket:  dockerSocket,
		socketConfigs: configs,
		configMu:      mu,
	}
}

// ServeHTTP handles HTTP requests to the proxy server
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get the socket path from the server name
	socketPath := h.dockerSocket

	// Forward the request to the Docker socket
	h.ServeHTTPWithSocket(w, r, socketPath)
}

// ServeHTTPWithSocket forwards the request to the Docker socket
func (h *ProxyHandler) ServeHTTPWithSocket(w http.ResponseWriter, r *http.Request, socketPath string) {
	log := logging.GetLogger()

	// Get the socket configuration
	h.configMu.RLock()
	socketConfig, ok := h.socketConfigs[socketPath]
	h.configMu.RUnlock()

	if !ok {
		log.Error("Socket configuration not found", "socket", socketPath)
		http.Error(w, "Socket configuration not found", http.StatusInternalServerError)
		return
	}

	// Check if the request is allowed by the ACLs
	allowed, reason := h.checkACLs(r, socketConfig)
	if !allowed {
		log.Warn("Request denied by ACL",
			"method", r.Method,
			"path", r.URL.Path,
			"socket", socketPath,
			"reason", reason,
		)
		http.Error(w, fmt.Sprintf("Request denied: %s", reason), http.StatusForbidden)
		return
	}

	// Create a reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// The URL will be used by the transport
			req.URL.Scheme = "http"
			req.URL.Host = "docker"
		},
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", h.dockerSocket)
			},
		},
	}

	// Serve the request
	proxy.ServeHTTP(w, r)
}

// checkACLs checks if a request is allowed by the ACLs
func (h *ProxyHandler) checkACLs(r *http.Request, socketConfig *config.SocketConfig) (bool, string) {
	log := logging.GetLogger()

	// If there's no config, allow all requests
	if socketConfig == nil {
		return true, ""
	}

	// If there are no ACLs, deny by default
	if len(socketConfig.Rules.ACLs) == 0 {
		return false, "no ACLs defined"
	}

	// Log the request details for debugging
	log.Info("Checking ACL rules",
		"path", r.URL.Path,
		"method", r.Method,
		"num_rules", len(socketConfig.Rules.ACLs))

	// Check each ACL rule
	for i, rule := range socketConfig.Rules.ACLs {
		// Log the rule being checked
		log.Info("Checking rule",
			"index", i,
			"path", rule.Match.Path,
			"method", rule.Match.Method,
			"action", rule.Action)

		// Check if the rule matches the request
		matched := h.ruleMatches(r, rule.Match)
		log.Info("Rule match result",
			"index", i,
			"matched", matched,
			"path_pattern", rule.Match.Path,
			"method_pattern", rule.Match.Method)

		if matched {
			log.Info("Rule matched",
				"index", i,
				"action", rule.Action,
				"reason", rule.Reason)

			if rule.Action == "allow" {
				return true, ""
			} else {
				return false, rule.Reason
			}
		}
	}

	// If no rule matches, deny by default
	log.Info("No matching rules found, denying by default")
	return false, "No matching allow rules"
}

// ruleMatches checks if a request matches an ACL rule
func (h *ProxyHandler) ruleMatches(r *http.Request, match config.Match) bool {
	log := logging.GetLogger()

	// Check path match
	if match.Path != "" {
		pathMatched, err := regexp.MatchString(match.Path, r.URL.Path)
		if err != nil {
			log.Error("Invalid regex pattern for path", "pattern", match.Path, "error", err)
			return false
		}
		if !pathMatched {
			log.Info("Path did not match",
				"request_path", r.URL.Path,
				"pattern", match.Path)
			return false
		}
		log.Info("Path matched",
			"request_path", r.URL.Path,
			"pattern", match.Path)
	}

	// Check method match
	if match.Method != "" {
		methodMatched, err := regexp.MatchString(match.Method, r.Method)
		if err != nil {
			log.Error("Invalid regex pattern for method", "pattern", match.Method, "error", err)
			return false
		}
		if !methodMatched {
			log.Info("Method did not match",
				"request_method", r.Method,
				"pattern", match.Method)
			return false
		}
		log.Info("Method matched",
			"request_method", r.Method,
			"pattern", match.Method)
	}

	// Check contains criteria if specified
	if len(match.Contains) > 0 && r.Method == "POST" {
		// Only check contains for POST requests that might have a body

		// Read the request body
		if r.Body == nil {
			log.Info("Request has no body, contains check failed")
			return false
		}

		// Read and restore the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error("Failed to read request body", "error", err)
			return false
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse the JSON body
		var body map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			log.Error("Failed to parse JSON body", "error", err)
			return false
		}

		// Check if the request body matches the contains criteria
		if !matchesContains(body, match.Contains) {
			log.Info("Request body does not match contains criteria")
			return false
		}
		log.Info("Contains check passed")
	}

	return true
}

// matchesContains checks if the request body matches the contains criteria
func matchesContains(body map[string]interface{}, contains map[string]interface{}) bool {
	log := logging.GetLogger()

	for key, expectedValue := range contains {
		actualValue, exists := body[key]
		if !exists {
			log.Info("Field not found in request body", "field", key)
			return false
		}

		// If the expected value is a map, recurse into it
		if expectedMap, ok := expectedValue.(map[string]interface{}); ok {
			if actualMap, ok := actualValue.(map[string]interface{}); ok {
				if !matchesContains(actualMap, expectedMap) {
					log.Info("Nested field does not match", "field", key)
					return false
				}
			} else {
				log.Info("Field is not a map", "field", key, "type", reflect.TypeOf(actualValue))
				return false
			}
		} else if expectedArray, ok := expectedValue.([]interface{}); ok {
			// Handle array matching
			if actualArray, ok := actualValue.([]interface{}); ok {
				// Check if all items in expected are in actual
				for _, expectedItem := range expectedArray {
					found := false
					for _, actualItem := range actualArray {
						if containsValue(actualItem, expectedItem) {
							found = true
							break
						}
					}
					if !found {
						log.Info("Array does not contain expected item",
							"field", key,
							"expected_item", expectedItem)
						return false
					}
				}
			} else if expectedStr, ok := expectedValue.(string); ok {
				// Special case for string matching in arrays
				found := false
				for _, actualItem := range actualArray {
					if itemStr, ok := actualItem.(string); ok && strings.Contains(itemStr, expectedStr) {
						found = true
						break
					}
				}
				if !found {
					log.Info("Array does not contain string",
						"field", key,
						"expected", expectedStr)
					return false
				}
			} else {
				log.Info("Field is not an array", "field", key, "type", reflect.TypeOf(actualValue))
				return false
			}
		} else {
			// For simple values, use containsValue
			if !containsValue(actualValue, expectedValue) {
				log.Info("Field value does not match expected value",
					"field", key,
					"actual", actualValue,
					"expected", expectedValue)
				return false
			}
		}

		log.Info("Field matches expected value", "field", key)
	}

	return true
}

// containsValue checks if a value contains another value
// This handles various types including strings, arrays, and maps
func containsValue(actual, expected interface{}) bool {

	// Handle nil values
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	// Handle different types
	switch expectedVal := expected.(type) {
	case string:
		// Check if the string contains regex metacharacters
		hasRegexChars := strings.ContainsAny(expectedVal, "^$.*+?()[]{}|\\")

		// For string values
		if actualStr, ok := actual.(string); ok {
			if hasRegexChars {
				// Try as regex first
				matched, err := regexp.MatchString(expectedVal, actualStr)
				if err == nil && matched {
					return true
				}
				// Fall back to regular contains if regex fails
			}
			return strings.Contains(actualStr, expectedVal)
		}

		// For array values, check if any element matches
		if actualArray, ok := actual.([]interface{}); ok {
			for _, item := range actualArray {
				if itemStr, ok := item.(string); ok {
					if hasRegexChars {
						// Try as regex first
						matched, err := regexp.MatchString(expectedVal, itemStr)
						if err == nil && matched {
							return true
						}
						// Fall back to regular contains if regex fails
					}
					if strings.Contains(itemStr, expectedVal) {
						return true
					}
				}
			}
			return false
		}

		return false

	case []interface{}:
		// Check if actual is an array
		actualArray, ok := actual.([]interface{})
		if !ok {
			return false
		}

		// Empty array case
		if len(expectedVal) == 0 && len(actualArray) == 0 {
			return true
		}

		// Check if all expected items are in the actual array
		for _, expectedItem := range expectedVal {
			found := false
			for _, actualItem := range actualArray {
				if containsValue(actualItem, expectedItem) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true

	case map[string]interface{}:
		// Check if actual is a map
		actualMap, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}

		// Check if all expected key-value pairs are in the actual map
		for key, expectedMapVal := range expectedVal {
			actualMapVal, exists := actualMap[key]
			if !exists || !containsValue(actualMapVal, expectedMapVal) {
				return false
			}
		}
		return true

	default:
		// For other types, use direct equality
		return reflect.DeepEqual(actual, expected)
	}
}
