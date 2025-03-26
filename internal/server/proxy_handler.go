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
	"regexp"
	"strconv"
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
	allowed, reason := h.checkACLRules(r, socketConfig)
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
			if action.Action == "allow" {
				log.Info("Allow action found", "reason", action.Reason)
				return true, action.Reason
			} else if action.Action == "deny" {
				log.Info("Deny action found", "reason", action.Reason)
				return false, action.Reason
			}
			// Continue with next action if not allow/deny
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

		// Restore the request body
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
	r.Body.Close()

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
		if !config.MatchesRule(r, rule.Match) {
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
				if config.MatchesStructure(body, action.Contains) {
					if config.MergeStructure(body, action.Update, true) {
						modified = true
					}
				}
			case "upsert":
				if config.MergeStructure(body, action.Update, false) {
					modified = true
				}
			case "delete":
				if config.DeleteMatchingFields(body, action.Contains) {
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
