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
	"regexp"
	"strconv"
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

// applyRewriteRules applies rewrite rules to a request
func (s *Server) applyRewriteRules(socketPath string, r *http.Request) error {
	// Get the socket configuration
	s.configMu.RLock()
	socketConfig, ok := s.socketConfigs[socketPath]
	s.configMu.RUnlock()

	if !ok || socketConfig == nil {
		return nil // No config, no rewrites
	}

	// Check if there are any rewrite rules
	if len(socketConfig.Rules.Rewrites) == 0 {
		return nil
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
	r.Body.Close()

	// Parse the JSON body
	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the original body
		return fmt.Errorf("failed to parse JSON body: %w", err)
	}

	// Check each rewrite rule
	modified := false
	for _, rule := range socketConfig.Rules.Rewrites {
		// Check if the rule matches
		if !matchesRule(r, rule.Match) {
			continue
		}

		// Apply the rewrite actions
		if applyRewriteActions(body, rule.Actions) {
			modified = true
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
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse the JSON body
		var body map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			return false
		}

		// Check if the body matches the contains criteria
		if !matchesContains(body, match.Contains) {
			return false
		}
	}

	return true
}

// applyRewriteActions applies rewrite actions to a request body
func applyRewriteActions(body map[string]interface{}, actions []config.RewriteAction) bool {
	modified := false

	for _, action := range actions {
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

	return modified
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
		} else if !containsValue(actualValue, expectedValue) {
			return false
		}
	}

	return true
}

// mergeStructure merges a structure into a body
func mergeStructure(body map[string]interface{}, update map[string]interface{}, replace bool) bool {
	modified := false

	for key, updateValue := range update {
		// Handle nested maps
		if updateMap, ok := updateValue.(map[string]interface{}); ok {
			// If the key doesn't exist or isn't a map, create it
			if actualValue, exists := body[key]; !exists {
				body[key] = updateMap
				modified = true
			} else if actualMap, ok := actualValue.(map[string]interface{}); ok {
				// Recurse into the nested map
				if mergeStructure(actualMap, updateMap, replace) {
					modified = true
				}
			} else if replace {
				// Replace the value
				body[key] = updateMap
				modified = true
			}
		} else if updateArray, ok := updateValue.([]interface{}); ok {
			// Handle arrays (like Env)
			if key == "Env" {
				// Special handling for Env arrays
				if actualValue, exists := body[key]; !exists {
					// Key doesn't exist, create it
					body[key] = updateArray
					modified = true
				} else if actualArray, ok := actualValue.([]interface{}); ok {
					// Merge or replace the array
					if replace {
						// For replace, we need to check each item
						newArray := make([]interface{}, 0, len(actualArray))
						replaced := false

						for _, item := range actualArray {
							// Check if this item should be replaced
							shouldReplace := false
							for _, updateItem := range updateArray {
								if containsValue(item, updateItem) {
									shouldReplace = true
									break
								}
							}

							if shouldReplace {
								// Add the update items instead
								for _, updateItem := range updateArray {
									newArray = append(newArray, updateItem)
								}
								replaced = true
								break
							} else {
								// Keep the original item
								newArray = append(newArray, item)
							}
						}

						if replaced {
							body[key] = newArray
							modified = true
						} else if len(updateArray) > 0 {
							// If no items were replaced but we have updates, append them
							body[key] = append(actualArray, updateArray...)
							modified = true
						}
					} else {
						// For upsert, just append
						body[key] = append(actualArray, updateArray...)
						modified = true
					}
				} else if replace {
					// Replace the value
					body[key] = updateArray
					modified = true
				}
			} else {
				// Regular array handling
				if actualValue, exists := body[key]; !exists {
					// Key doesn't exist, create it
					body[key] = updateArray
					modified = true
				} else if actualArray, ok := actualValue.([]interface{}); ok {
					if replace {
						// Replace the array
						body[key] = updateArray
						modified = true
					} else {
						// Merge the arrays
						body[key] = append(actualArray, updateArray...)
						modified = true
					}
				} else if replace {
					// Replace the value
					body[key] = updateArray
					modified = true
				}
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
						if containsValue(item, matchItem) {
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
			if containsValue(actualValue, matchValue) {
				delete(body, key)
				modified = true
			}
		}
	}

	return modified
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
