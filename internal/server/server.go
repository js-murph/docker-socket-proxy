package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"
)

type Server struct {
	managementSocket string
	dockerSocket     string
	server           *http.Server
	socketConfigs    map[string]*config.SocketConfig
	configMu         sync.RWMutex
	createdSockets   []string // Track created socket paths
	socketMu         sync.RWMutex
}

type contextKey string

const serverContextKey contextKey = "server"

func New(paths *management.SocketPaths) *Server {
	if err := paths.Validate(); err != nil {
		panic(fmt.Sprintf("invalid socket paths: %v", err))
	}

	return &Server{
		managementSocket: paths.Management,
		dockerSocket:     paths.Docker,
		socketConfigs:    make(map[string]*config.SocketConfig),
		createdSockets:   make([]string, 0),
	}
}

// TrackSocket adds a socket path to the list of created sockets
func (s *Server) TrackSocket(path string) {
	s.socketMu.Lock()
	defer s.socketMu.Unlock()
	s.createdSockets = append(s.createdSockets, path)
}

// UntrackSocket removes a socket path from the list of created sockets
func (s *Server) UntrackSocket(path string) {
	s.socketMu.Lock()
	defer s.socketMu.Unlock()
	for i, p := range s.createdSockets {
		if p == path {
			s.createdSockets = append(s.createdSockets[:i], s.createdSockets[i+1:]...)
			break
		}
	}
}

func (s *Server) Start() error {
	log := logging.GetLogger()

	// Create signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize the server
	handler := NewManagementHandler(s.dockerSocket, s.socketConfigs, &s.configMu)
	listener, err := net.Listen("unix", s.managementSocket)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	s.server = &http.Server{
		Handler: handler,
	}

	// Start server
	go func() {
		if err := s.server.Serve(listener); err != http.ErrServerClosed {
			log.Error("Server error", "error", err)
		}
	}()

	log.Info("Management server started", "socket", s.managementSocket)

	// Wait for shutdown signal
	<-sigChan
	log.Info("Shutting down server...")

	// Cleanup all created sockets
	s.socketMu.Lock()
	for _, socketPath := range s.createdSockets {
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			log.Error("Failed to remove socket", "path", socketPath, "error", err)
		}
	}
	s.socketMu.Unlock()

	// Remove management socket
	if err := os.Remove(s.managementSocket); err != nil && !os.IsNotExist(err) {
		log.Error("Failed to remove management socket", "error", err)
	}

	// Shutdown server
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down server: %w", err)
	}

	return nil
}

func (s *Server) Stop() {
	if s.server != nil {
		s.server.Shutdown(context.Background())
	}
	s.cleanup()
}

func (s *Server) cleanup() {
	// Clean up management socket
	os.Remove(s.managementSocket)

	// Clean up all created sockets
	s.socketMu.Lock()
	for _, socketPath := range s.createdSockets {
		if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
			log := logging.GetLogger()
			log.Error("Failed to remove socket during cleanup",
				"path", socketPath,
				"error", err)
		}
	}
	s.createdSockets = nil
	s.socketMu.Unlock()
}

func matchesRule(r *http.Request, match config.Match) bool {
	// Path matching with wildcards
	if match.Path != "" && !config.MatchPattern(match.Path, r.URL.Path) {
		log := logging.GetLogger()
		log.Info("Path mismatch", "rule", match.Path, "request", r.URL.Path)
		return false
	}

	// Method matching (exact)
	if match.Method != "" && match.Method != r.Method {
		log := logging.GetLogger()
		log.Info("Method mismatch", "rule", match.Method, "request", r.Method)
		return false
	}

	// Contains matching with wildcards
	if len(match.Contains) > 0 {
		if r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" {
			log := logging.GetLogger()
			log.Info("Method not applicable for contains check", "method", r.Method)
			return false
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log := logging.GetLogger()
			log.Info("Error reading request body", "error", err)
			return false
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var requestBody map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
			log := logging.GetLogger()
			log.Info("Error parsing request body as JSON", "error", err)
			return false
		}

		log := logging.GetLogger()
		log.Info("Request body", "body", string(bodyBytes))
		log.Info("Contains rules", "rules", match.Contains)

		// Check each field in Contains against the request body
		for key, expectedValue := range match.Contains {
			actualValue, exists := deepGet(requestBody, strings.Split(key, "."))
			if !exists {
				log.Info("Key not found in request", "key", key)
				return false
			}

			if !config.MatchValue(expectedValue, actualValue) {
				log.Info("Value mismatch for key", "key", key)
				return false
			}
		}
	}

	log := logging.GetLogger()
	log.Info("Rule matched successfully")
	return true
}

// deepGet traverses a nested map using the given path and returns the value
func deepGet(m map[string]interface{}, path []string) (interface{}, bool) {
	if len(path) == 0 {
		return nil, false
	}

	if len(path) == 1 {
		val, exists := m[path[0]]
		return val, exists
	}

	if next, ok := m[path[0]].(map[string]interface{}); ok {
		return deepGet(next, path[1:])
	}

	return nil, false
}

func (s *Server) applyRewriteRules(socketPath string, r *http.Request) error {
	s.configMu.RLock()
	config, exists := s.socketConfigs[socketPath]
	s.configMu.RUnlock()

	if !exists || config == nil || len(config.Rules.Rewrites) == 0 {
		return nil // No rewrites needed
	}

	// Only handle requests with bodies
	if r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" {
		return nil
	}

	// Skip if no body or not JSON content type
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Skip empty bodies
	if len(bodyBytes) == 0 {
		return nil
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		return nil // Skip rewriting if not valid JSON
	}

	modified := false
	// Check each rewrite rule
	for _, rule := range config.Rules.Rewrites {
		if matchesRule(r, rule.Match) {
			// Apply each pattern in the rule
			for _, pattern := range rule.Patterns {
				if rewritten := applyPattern(requestBody, pattern); rewritten {
					modified = true
				}
			}
		}
	}

	// If body was modified, replace it
	if modified {
		newBody, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshaling modified body: %w", err)
		}
		r.Body = io.NopCloser(bytes.NewBuffer(newBody))
		r.ContentLength = int64(len(newBody))
	}

	return nil
}

func applyPattern(body map[string]interface{}, pattern config.Pattern) bool {
	slog.Debug("applying pattern",
		"field", pattern.Field,
		"action", pattern.Action,
		"match", pattern.Match,
		"value", pattern.Value)

	path := strings.Split(pattern.Field, ".")

	// Handle delete action
	if pattern.Action == "delete" {
		value, exists := deepGet(body, path)
		if !exists {
			slog.Debug("delete: field not found", "path", pattern.Field)
			return false
		}
		slog.Debug("delete: found value",
			"path", pattern.Field,
			"value", value)

		// If no match specified, delete the entire field
		if pattern.Match == nil {
			if len(path) == 1 {
				delete(body, path[0])
				return true
			}
			parent, exists := deepGet(body, path[:len(path)-1])
			if !exists {
				return false
			}
			if parentMap, ok := parent.(map[string]interface{}); ok {
				delete(parentMap, path[len(path)-1])
				return true
			}
			return false
		}

		// Handle array types specially
		if arr, ok := value.([]interface{}); ok {
			return deleteFromArray(body, path, arr, pattern)
		}

		// Handle simple value types
		switch v := value.(type) {
		case bool:
			if matchBool, ok := pattern.Match.(bool); ok && matchBool == v {
				if len(path) == 1 {
					delete(body, path[0])
					return true
				}
				parent, _ := deepGet(body, path[:len(path)-1])
				if parentMap, ok := parent.(map[string]interface{}); ok {
					delete(parentMap, path[len(path)-1])
					return true
				}
			}
		case string:
			if matchStr, ok := pattern.Match.(string); ok && config.MatchPattern(matchStr, v) {
				if len(path) == 1 {
					delete(body, path[0])
					return true
				}
				parent, _ := deepGet(body, path[:len(path)-1])
				if parentMap, ok := parent.(map[string]interface{}); ok {
					delete(parentMap, path[len(path)-1])
					return true
				}
			}
		}
		return false
	}

	value, exists := deepGet(body, path)
	if !exists {
		slog.Debug("field not found", "path", pattern.Field)
		if pattern.Action == "upsert" {
			slog.Debug("upserting new value", "value", pattern.Value)
			return deepSet(body, path, pattern.Value)
		}
		return false
	}

	slog.Debug("found existing value", "value", value)
	// Handle different types of matches
	switch v := value.(type) {
	case []interface{}:
		return rewriteArray(body, path, v, pattern)
	case bool:
		if pattern.Match == v {
			return deepSet(body, path, pattern.Value)
		}
	case string:
		if config.MatchPattern(pattern.Match.(string), v) {
			return deepSet(body, path, pattern.Value)
		}
	}

	return false
}

func rewriteArray(body map[string]interface{}, path []string, arr []interface{}, pattern config.Pattern) bool {
	slog.Debug("rewriting array",
		"path", strings.Join(path, "."),
		"array", arr)

	modified := false

	if path[len(path)-1] == "Env" {
		slog.Debug("handling environment variables")
		return rewriteEnvVars(body, path, arr, pattern)
	}

	// Handle other array types
	for i, item := range arr {
		str, ok := item.(string)
		if !ok {
			slog.Debug("skipping non-string item",
				"index", i,
				"item", item)
			continue
		}
		if config.MatchPattern(pattern.Match.(string), str) {
			slog.Debug("matched item",
				"index", i,
				"old", str,
				"new", pattern.Value)
			arr[i] = pattern.Value
			modified = true
		}
	}

	if modified {
		slog.Debug("array modified", "final", arr)
		return deepSet(body, path, arr)
	}
	slog.Debug("no modifications made to array")
	return false
}

func rewriteEnvVars(body map[string]interface{}, path []string, arr []interface{}, pattern config.Pattern) bool {
	slog.Debug("processing environment variables",
		"action", pattern.Action,
		"vars", arr)

	valueStr, ok := pattern.Value.(string)
	if !ok && pattern.Action != "delete" {
		slog.Debug("invalid value type for non-delete action",
			"type", fmt.Sprintf("%T", pattern.Value))
		return false
	}

	if pattern.Action == "upsert" {
		keyPart := strings.Split(valueStr, "=")[0]
		slog.Debug("upserting env var", "key", keyPart)
		modified := false
		newArr := make([]interface{}, 0, len(arr))

		found := false
		for _, item := range arr {
			str, ok := item.(string)
			if !ok {
				newArr = append(newArr, item)
				continue
			}

			if strings.HasPrefix(str, keyPart+"=") {
				newArr = append(newArr, valueStr)
				found = true
				modified = true
			} else {
				newArr = append(newArr, str)
			}
		}

		// If not found, append it
		if !found {
			newArr = append(newArr, valueStr)
			modified = true
		}

		if modified {
			slog.Debug("environment variables modified",
				"new_vars", newArr)
			return deepSet(body, path, newArr)
		}
		return false
	}

	// Handle replace and delete actions
	matchStr, ok := pattern.Match.(string)
	if !ok && pattern.Action != "delete" {
		slog.Debug("invalid match type for non-delete action",
			"type", fmt.Sprintf("%T", pattern.Match))
		return false
	}

	modified := false
	newArr := make([]interface{}, 0, len(arr))

	for i, item := range arr {
		str, ok := item.(string)
		if !ok {
			slog.Debug("skipping non-string item",
				"index", i,
				"item", item)
			newArr = append(newArr, item)
			continue
		}

		if pattern.Action == "delete" {
			if !config.MatchPattern(matchStr, str) {
				newArr = append(newArr, str)
			} else {
				slog.Debug("deleting env var", "var", str)
				modified = true
			}
		} else if config.MatchPattern(matchStr, str) {
			slog.Debug("replacing env var",
				"old", str,
				"new", valueStr)
			newArr = append(newArr, valueStr)
			modified = true
		} else {
			newArr = append(newArr, str)
		}
	}

	if modified {
		slog.Debug("environment variables modified", "final", newArr)
		return deepSet(body, path, newArr)
	}
	slog.Debug("no modifications made to environment variables")
	return false
}

func deepSet(m map[string]interface{}, path []string, value interface{}) bool {
	if len(path) == 0 {
		return false
	}

	if len(path) == 1 {
		m[path[0]] = value
		return true
	}

	next, ok := m[path[0]].(map[string]interface{})
	if !ok {
		next = make(map[string]interface{})
		m[path[0]] = next
	}

	return deepSet(next, path[1:], value)
}

func deleteFromArray(body map[string]interface{}, path []string, arr []interface{}, pattern config.Pattern) bool {
	slog.Debug("deleting from array",
		"path", strings.Join(path, "."),
		"array", arr)

	if path[len(path)-1] == "Env" {
		slog.Debug("handling environment variables deletion")
		return deleteEnvVars(body, path, arr, pattern)
	}

	modified := false
	newArr := make([]interface{}, 0, len(arr))

	for i, item := range arr {
		switch v := item.(type) {
		case string:
			if matchStr, ok := pattern.Match.(string); ok {
				if !config.MatchPattern(matchStr, v) {
					newArr = append(newArr, item)
				} else {
					slog.Debug("deleting string item",
						"index", i,
						"value", v)
					modified = true
				}
			}
		case bool:
			if matchBool, ok := pattern.Match.(bool); ok {
				if matchBool != v {
					newArr = append(newArr, item)
				} else {
					slog.Debug("deleting boolean item",
						"index", i,
						"value", v)
					modified = true
				}
			}
		default:
			slog.Debug("skipping unsupported type",
				"index", i,
				"type", fmt.Sprintf("%T", item))
			newArr = append(newArr, item)
		}
	}

	if modified {
		slog.Debug("array modified", "final", newArr)
		return deepSet(body, path, newArr)
	}
	slog.Debug("no modifications made to array")
	return false
}

func deleteEnvVars(body map[string]interface{}, path []string, arr []interface{}, pattern config.Pattern) bool {
	slog.Debug("deleting environment variables",
		"path", strings.Join(path, "."),
		"vars", arr)

	modified := false

	// For upsert, first try to find and replace existing value
	if pattern.Action == "upsert" {
		keyPart := strings.Split(pattern.Value.(string), "=")[0]
		newArr := make([]interface{}, 0, len(arr))

		found := false
		for _, item := range arr {
			str, ok := item.(string)
			if !ok {
				newArr = append(newArr, item)
				continue
			}

			if strings.HasPrefix(str, keyPart+"=") {
				found = true
				modified = true
			} else {
				newArr = append(newArr, str)
			}
		}

		// If not found, append it
		if !found {
			newArr = append(newArr, pattern.Value)
			modified = true
		}

		if modified {
			slog.Debug("environment variables modified",
				"new_vars", newArr)
			return deepSet(body, path, newArr)
		}
		return false
	}

	// Handle replace and delete actions
	matchStr, ok := pattern.Match.(string)
	if !ok && pattern.Action != "delete" {
		return false
	}

	modified = false
	newArr := make([]interface{}, 0, len(arr))

	for _, item := range arr {
		str, ok := item.(string)
		if !ok {
			newArr = append(newArr, item)
			continue
		}

		if pattern.Action == "delete" {
			if !config.MatchPattern(matchStr, str) {
				newArr = append(newArr, str)
			} else {
				slog.Debug("deleting env var", "var", str)
				modified = true
			}
		} else if config.MatchPattern(matchStr, str) {
			modified = true
		} else {
			newArr = append(newArr, str)
		}
	}

	if modified {
		slog.Debug("environment variables modified",
			"new_vars", newArr)
		return deepSet(body, path, newArr)
	}
	return false
}
