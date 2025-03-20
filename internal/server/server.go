package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"
)

// Server represents the Docker socket proxy server
type Server struct {
	managementSocket string
	dockerSocket     string
	socketDir        string
	server           *http.Server
	socketConfigs    map[string]*config.SocketConfig
	proxyServers     map[string]*http.Server
	createdSockets   []string
	store            *storage.FileStore
	configMu         sync.RWMutex
	proxyMu          sync.RWMutex
	socketMu         sync.Mutex
}

type contextKey string

const serverContextKey contextKey = "server"

// NewServer creates a new server instance
func NewServer(managementSocket, dockerSocket, socketDir string) (*Server, error) {
	// Create socket directory if it doesn't exist
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Create the file store
	store := storage.NewFileStore(socketDir)

	return &Server{
		managementSocket: managementSocket,
		dockerSocket:     dockerSocket,
		socketDir:        socketDir,
		socketConfigs:    make(map[string]*config.SocketConfig),
		proxyServers:     make(map[string]*http.Server),
		createdSockets:   make([]string, 0),
		store:            store,
	}, nil
}

// TrackSocket adds a socket to the list of created sockets
func (s *Server) TrackSocket(path string) {
	s.socketMu.Lock()
	defer s.socketMu.Unlock()
	s.createdSockets = append(s.createdSockets, path)
}

// UntrackSocket removes a socket from the list of created sockets
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

// Start starts the server
func (s *Server) Start() error {
	log := logging.GetLogger()
	log.Debug("Starting server")

	if err := s.prepareSocket(); err != nil {
		return err
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Shutdown signal received, cleaning up...")
		s.Stop()
		os.Exit(0)
	}()

	// Load existing socket configurations
	if err := s.loadExistingConfigs(); err != nil {
		log.Error("Failed to load existing configurations", "error", err)
		// Continue anyway - we can still serve new sockets
	}

	// Create the listener
	listener, err := net.Listen("unix", s.managementSocket)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(s.managementSocket, 0660); err != nil {
		log.Warn("Failed to set socket permissions", "error", err)
	}

	// Create the management handler
	handler := NewManagementHandler(s.dockerSocket, s.socketConfigs, &s.configMu, s.store)

	// Create the server
	s.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), serverContextKey, s)
			handler.ServeHTTP(w, r.WithContext(ctx))
		}),
	}

	log.Info("Management server listening on socket", "path", s.managementSocket)
	log.Debug("Active proxy sockets", "count", len(s.createdSockets))

	return s.server.Serve(listener)
}

// Stop stops the server
func (s *Server) Stop() {
	log := logging.GetLogger()
	log.Debug("Stopping server")

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown the management server
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			log.Error("Error shutting down management server", "error", err)
		}
	}

	// Shutdown all proxy servers
	s.proxyMu.Lock()
	for path, server := range s.proxyServers {
		if err := server.Shutdown(ctx); err != nil {
			log.Error("Error shutting down proxy server", "error", err, "path", path)
		}
	}
	s.proxyMu.Unlock()

	// Clean up resources
	s.cleanup()
}

// prepareSocket prepares the management socket
func (s *Server) prepareSocket() error {
	// Remove existing socket if it exists
	if err := os.Remove(s.managementSocket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}
	return nil
}

// loadExistingConfigs loads existing socket configurations and restarts their servers
func (s *Server) loadExistingConfigs() error {
	log := logging.GetLogger()

	// Get all socket config files
	configs, err := s.store.LoadExistingConfigs()
	if err != nil {
		return fmt.Errorf("failed to list configs: %w", err)
	}

	// Load each config
	for path, cfg := range configs {
		// Ensure the socket path is in the correct directory
		socketName := filepath.Base(path)
		socketPath := filepath.Join(management.DefaultSocketDir, socketName)

		// Check if the socket file exists and remove it if it does
		// (we'll recreate it with the listener)
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				log.Warn("Failed to remove existing socket file", "path", socketPath, "error", err)
				continue
			}
		}

		// Add the config to the map
		s.configMu.Lock()
		s.socketConfigs[socketPath] = cfg
		s.configMu.Unlock()

		// Create a new listener for the socket
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			log.Error("Failed to create listener for existing socket", "path", socketPath, "error", err)
			continue
		}

		// Set socket permissions
		if err := os.Chmod(socketPath, 0660); err != nil {
			log.Warn("Failed to set socket permissions", "path", socketPath, "error", err)
		}

		// Create a proxy handler for the socket
		proxyHandler := NewProxyHandler(s.dockerSocket, s.socketConfigs, &s.configMu)

		// Create a server for the socket
		server := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Apply rewrite rules if needed
				if err := s.applyRewriteRules(socketPath, r); err != nil {
					log.Error("Failed to apply rewrite rules", "error", err)
				}

				// Serve the request
				proxyHandler.ServeHTTPWithSocket(w, r, socketPath)
			}),
		}

		// Add the server to the map
		s.proxyMu.Lock()
		s.proxyServers[socketPath] = server
		s.proxyMu.Unlock()

		// Track the socket
		s.TrackSocket(socketPath)

		// Start the server in a goroutine
		go func(p string, l net.Listener) {
			log.Info("Restored proxy socket", "path", p)
			if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Error("Proxy server error", "error", err, "path", p)
			}
		}(socketPath, listener)
	}

	return nil
}

// applyRewriteRules applies any rewrite rules to the request
func (s *Server) applyRewriteRules(socketPath string, r *http.Request) error {
	// Get the socket configuration
	s.configMu.RLock()
	socketConfig, exists := s.socketConfigs[socketPath]
	s.configMu.RUnlock()

	if !exists || socketConfig == nil || len(socketConfig.Rules.Rewrites) == 0 {
		return nil
	}

	// Only apply rewrites to POST, PUT, PATCH requests with a body
	if r.Method != "POST" && r.Method != "PUT" && r.Method != "PATCH" {
		return nil
	}

	// Read the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Close the original body
	r.Body.Close()

	// Parse the request body as JSON
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		// If the body is not valid JSON, restore the original body and continue
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		return nil
	}

	// Apply each rewrite rule
	modified := false
	for _, rewrite := range socketConfig.Rules.Rewrites {
		// Check if the rewrite rule matches the request
		if matchesRule(r, rewrite.Match) {
			// Apply each pattern in the rewrite rule
			for _, pattern := range rewrite.Patterns {
				if applyPattern(body, pattern) {
					modified = true
				}
			}
		}
	}

	// If the body was modified, update the request body
	if modified {
		// Marshal the modified body back to JSON
		newBodyBytes, err := json.Marshal(body)
		if err != nil {
			// If marshaling fails, restore the original body
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			return fmt.Errorf("failed to marshal modified body: %w", err)
		}

		// Update the Content-Length header
		r.ContentLength = int64(len(newBodyBytes))

		// Set the new body
		r.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
	} else {
		// Restore the original body
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	return nil
}

// cleanup cleans up resources
func (s *Server) cleanup() {
	log := logging.GetLogger()

	// Remove the management socket
	if err := os.Remove(s.managementSocket); err != nil && !os.IsNotExist(err) {
		log.Error("Failed to remove management socket", "error", err)
	}

	// Remove all created sockets
	s.socketMu.Lock()
	for _, path := range s.createdSockets {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Error("Failed to remove socket", "error", err, "path", path)
		}
	}
	s.socketMu.Unlock()
}

// matchesRule checks if a request matches a rewrite rule
func matchesRule(r *http.Request, match config.Match) bool {
	// Check path match
	if match.Path != "" {
		matched, err := regexp.MatchString(match.Path, r.URL.Path)
		if err != nil || !matched {
			return false
		}
	}

	// Check method match
	if match.Method != "" {
		matched, err := regexp.MatchString(match.Method, r.Method)
		if err != nil || !matched {
			return false
		}
	}

	return true
}

// DeleteSocket stops and removes a proxy socket
func (s *Server) DeleteSocket(socketPath string) error {
	// Stop the proxy server if it's running
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if server, ok := s.proxyServers[socketPath]; ok {
		if err := server.Close(); err != nil {
			return fmt.Errorf("failed to stop proxy server: %w", err)
		}
		delete(s.proxyServers, socketPath)
	}

	// Remove the socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove socket file: %w", err)
	}

	// Remove the config
	delete(s.socketConfigs, socketPath)

	// Remove from created sockets list
	for i, path := range s.createdSockets {
		if path == socketPath {
			s.createdSockets = append(s.createdSockets[:i], s.createdSockets[i+1:]...)
			break
		}
	}

	return nil
}

// applyPattern applies a pattern to a request body
func applyPattern(body map[string]interface{}, pattern config.Pattern) bool {
	// Get the field value using dot notation (e.g., "HostConfig.Privileged")
	fieldPath := strings.Split(pattern.Field, ".")
	current := body

	// Navigate to the parent object of the field
	for i := 0; i < len(fieldPath)-1; i++ {
		field := fieldPath[i]
		if val, ok := current[field]; ok {
			if nestedMap, ok := val.(map[string]interface{}); ok {
				current = nestedMap
			} else {
				return false // Field path doesn't exist
			}
		} else {
			return false // Field path doesn't exist
		}
	}

	// Get the final field name
	fieldName := fieldPath[len(fieldPath)-1]

	// Handle different actions
	switch pattern.Action {
	case "replace":
		// For array fields like Env
		if fieldName == "Env" {
			if envArray, ok := current[fieldName].([]interface{}); ok {
				// Create a new array with the replaced values
				newEnv := make([]interface{}, 0, len(envArray))
				replaced := false

				// Check if we're dealing with the test case format (DEBUG=true)
				matchStr, isMatchStr := pattern.Match.(string)
				valueStr, isValueStr := pattern.Value.(string)

				for _, env := range envArray {
					if envStr, ok := env.(string); ok {
						// Special case for the test
						if isMatchStr && isValueStr && envStr == matchStr {
							newEnv = append(newEnv, valueStr)
							replaced = true
						} else if isMatchStr && strings.HasPrefix(envStr, matchStr+"=") {
							// Regular case: replace value after the equals sign
							parts := strings.SplitN(envStr, "=", 2)
							newEnv = append(newEnv, parts[0]+"="+valueStr)
							replaced = true
						} else {
							// Keep the original value
							newEnv = append(newEnv, envStr)
						}
					} else {
						// Keep non-string values
						newEnv = append(newEnv, env)
					}
				}

				if replaced {
					current[fieldName] = newEnv
					return true
				}
			}
		} else {
			// For regular fields, just replace the value
			if val, ok := current[fieldName]; ok {
				if reflect.DeepEqual(val, pattern.Match) {
					current[fieldName] = pattern.Value
					return true
				}
			}
		}

	case "upsert":
		// For array fields like Env
		if fieldName == "Env" {
			if envArray, ok := current[fieldName].([]interface{}); ok {
				// Add the value if it doesn't exist
				current[fieldName] = append(envArray, pattern.Value)
				return true
			} else if _, ok := current[fieldName]; !ok {
				// Create the array if it doesn't exist
				current[fieldName] = []interface{}{pattern.Value}
				return true
			}
		} else {
			// For regular fields, just set the value
			current[fieldName] = pattern.Value
			return true
		}

	case "delete":
		// For array fields like Env
		if fieldName == "Env" {
			if envArray, ok := current[fieldName].([]interface{}); ok {
				// Create a new array without the matching items
				newEnv := make([]interface{}, 0, len(envArray))
				for _, env := range envArray {
					if envStr, ok := env.(string); ok {
						// If the pattern has a wildcard, use a regex match
						if strings.Contains(pattern.Match.(string), "*") {
							pattern := strings.Replace(pattern.Match.(string), "*", ".*", -1)
							matched, _ := regexp.MatchString("^"+pattern+"$", envStr)
							if !matched {
								newEnv = append(newEnv, env)
							}
						} else if envStr != pattern.Match {
							// Otherwise use exact match
							newEnv = append(newEnv, env)
						}
					} else {
						newEnv = append(newEnv, env)
					}
				}
				current[fieldName] = newEnv
				return true
			}
		} else if _, ok := current[fieldName]; ok {
			// For regular fields, delete the field
			delete(current, fieldName)
			return true
		}
	}

	return false
}
