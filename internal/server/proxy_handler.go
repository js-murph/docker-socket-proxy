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
	reverseProxy  *httputil.ReverseProxy
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

		// Check each contains criteria
		for field, expectedValue := range match.Contains {
			// Get the actual value from the body
			actualValue, exists := getNestedValue(body, field)
			if !exists {
				log.Info("Field not found in request body", "field", field)
				return false
			}

			// Check if the actual value contains the expected value
			if !containsValue(actualValue, expectedValue) {
				log.Info("Field value does not contain expected value",
					"field", field,
					"actual", actualValue,
					"expected", expectedValue)
				return false
			}
			log.Info("Contains check passed",
				"field", field,
				"actual", actualValue,
				"expected", expectedValue)
		}
	}

	return true
}

// getNestedValue gets a nested value from a map using dot notation
func getNestedValue(data map[string]interface{}, path string) (interface{}, bool) {
	log := logging.GetLogger()
	parts := strings.Split(path, ".")
	current := data

	log.Debug("Getting nested value", "path", path, "data_keys", getMapKeys(current))

	// Navigate through the nested structure
	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part, return the value
			val, exists := current[part]
			if exists {
				log.Debug("Found value", "path", path, "value", val)
			} else {
				log.Debug("Value not found", "path", path, "current_keys", getMapKeys(current))
			}
			return val, exists
		}

		// Not the last part, navigate deeper
		next, exists := current[part]
		if !exists {
			log.Debug("Path segment not found", "segment", part, "current_keys", getMapKeys(current))
			return nil, false
		}

		// Check if the next level is a map
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			// Try to convert from json.RawMessage or other types
			log.Debug("Not a map, trying to convert", "type", reflect.TypeOf(next))

			// If it's a slice of interfaces, check if it contains maps
			if sliceVal, isSlice := next.([]interface{}); isSlice {
				// For Docker API, Env is often a slice of strings
				if part == "Env" {
					return sliceVal, true
				}
			}

			return nil, false
		}

		current = nextMap
	}

	return nil, false
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// containsValue checks if a value contains another value
// This handles various types including strings, arrays, and maps
func containsValue(actual interface{}, expected interface{}) bool {
	log := logging.GetLogger()

	// Handle nil values
	if actual == nil {
		return expected == nil
	}

	log.Debug("Checking contains",
		"actual_type", reflect.TypeOf(actual),
		"expected_type", reflect.TypeOf(expected))

	// Handle different types
	switch actualVal := actual.(type) {
	case []interface{}:
		// For arrays, check if any element matches the expected value

		// Special handling for Docker API Env variables which are strings like "KEY=VALUE"
		if expectedSlice, ok := expected.([]interface{}); ok {
			// If expected is also a slice, check if all items in expected are in actual
			for _, expectedItem := range expectedSlice {
				found := false
				for _, actualItem := range actualVal {
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
		}

		// If expected is a single value, check if it's in the array
		for _, item := range actualVal {
			if containsValue(item, expected) {
				return true
			}
		}
		return false

	case string:
		// For strings, check if it contains the expected value as a substring
		if expectedStr, ok := expected.(string); ok {
			// For Docker API Env variables which are in format "KEY=VALUE"
			if strings.Contains(actualVal, expectedStr) {
				log.Debug("String contains match", "actual", actualVal, "expected", expectedStr)
				return true
			}
		}
		return actual == expected

	default:
		// For other types, check for equality
		return reflect.DeepEqual(actual, expected)
	}
}
