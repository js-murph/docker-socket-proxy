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

	// Process rules and apply rewrites in a single pass
	allowed, reason, err := h.processRules(r, socketConfig)
	if err != nil {
		log.Error("Error processing rules", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

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
			req.URL.Scheme = "http"
			req.URL.Host = "docker"
		},
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", h.dockerSocket)
			},
		},
	}

	proxy.ServeHTTP(w, r)
}

// processRules handles both ACL checks and rewrites in a single pass
func (h *ProxyHandler) processRules(r *http.Request, socketConfig *config.SocketConfig) (allowed bool, reason string, err error) {
	log := logging.GetLogger()

	// Handle nil config - allow by default
	if socketConfig == nil {
		return true, "", nil
	}

	// If there are no rules, allow by default
	if len(socketConfig.Rules) == 0 {
		return true, "", nil
	}

	// For POST/PUT requests that might need rewrites
	var bodyBytes []byte
	var body map[string]any
	modified := false

	if r.Method == "POST" || r.Method == "PUT" && r.Body != nil {
		// Read the body
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			return false, "", fmt.Errorf("failed to read request body: %w", err)
		}

		// Create a new reader for the body immediately
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Try to parse JSON body
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			// If we can't parse JSON, that's ok - we'll just use the original body
			body = nil
		}
	}

	// Process each rule in order
	for _, rule := range socketConfig.Rules {
		// Check path and method matches
		pathMatches := true
		if rule.Match.Path != "" {
			pathMatches, err = regexp.MatchString(rule.Match.Path, r.URL.Path)
			if err != nil {
				return false, "", fmt.Errorf("invalid path pattern: %w", err)
			}
		}
		if !pathMatches {
			log.Debug("Path does not match", "path", r.URL.Path, "pattern", rule.Match.Path)
			continue
		}

		methodMatches := true
		if rule.Match.Method != "" {
			methodMatches, err = regexp.MatchString(rule.Match.Method, r.Method)
			if err != nil {
				return false, "", fmt.Errorf("invalid method pattern: %w", err)
			}
		}
		if !methodMatches {
			log.Debug("Method does not match", "method", r.Method, "pattern", rule.Match.Method)
			continue
		}

		// Check rule's Contains condition
		if len(rule.Match.Contains) > 0 {
			if body == nil {
				log.Debug("No body available for Contains check")
				continue
			}
			if !config.MatchValue(rule.Match.Contains, body) {
				log.Debug("Body does not match Contains condition", "contains", rule.Match.Contains)
				continue
			}
		}

		log.Debug("Rule matched", "path", r.URL.Path, "method", r.Method)

		// Rule matches, now process its actions
		for _, action := range rule.Actions {
			switch action.Action {
			case "deny":
				if len(action.Contains) > 0 && body != nil {
					if !config.MatchValue(action.Contains, body) {
						continue
					}
				}
				return false, action.Reason, nil

			case "allow":
				if modified && body != nil {
					// Update the body if it was modified
					newBodyBytes, err := json.Marshal(body)
					if err != nil {
						return false, "", fmt.Errorf("failed to marshal modified body: %w", err)
					}
					r.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
					r.ContentLength = int64(len(newBodyBytes))
					r.Header.Set("Content-Length", strconv.Itoa(len(newBodyBytes)))
				} else {
					// Restore original body
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					r.ContentLength = int64(len(bodyBytes))
					r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
				}
				return true, action.Reason, nil

			case "replace":
				if body != nil && config.MatchesStructure(body, action.Contains) {
					if config.MergeStructure(body, action.Update, true) {
						modified = true
					}
				}

			case "upsert":
				if body != nil {
					if config.MergeStructure(body, action.Update, false) {
						modified = true
					}
				}

			case "delete":
				if body != nil {
					if config.DeleteMatchingFields(body, action.Contains) {
						modified = true
					}
				}
			}
		}
	}

	// If we get here, no explicit allow/deny was found
	// Restore the body and allow by default
	if modified && body != nil {
		newBodyBytes, err := json.Marshal(body)
		if err != nil {
			return false, "", fmt.Errorf("failed to marshal modified body: %w", err)
		}
		r.Body = io.NopCloser(bytes.NewBuffer(newBodyBytes))
		r.ContentLength = int64(len(newBodyBytes))
		r.Header.Set("Content-Length", strconv.Itoa(len(newBodyBytes)))
	} else if bodyBytes != nil {
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	}

	return true, "", nil
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
		var bodyJSON map[string]any
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
