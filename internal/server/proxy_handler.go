package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

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

// ServeHTTP handles HTTP requests by checking ACLs and proxying to Docker
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This is for backward compatibility with existing code
	h.ServeHTTPWithSocket(w, r, h.dockerSocket)
}

// ServeHTTPWithSocket handles HTTP requests for a specific socket
func (h *ProxyHandler) ServeHTTPWithSocket(w http.ResponseWriter, r *http.Request, socketPath string) {
	log := logging.GetLogger()
	start := time.Now()

	// Get the socket configuration
	h.configMu.RLock()
	socketConfig, _ := h.socketConfigs[socketPath]
	h.configMu.RUnlock()

	// Check if the request is allowed by the ACLs
	allowed, reason := h.checkACLs(r, socketConfig)

	// Log the request
	log.Info("Proxy request",
		"method", r.Method,
		"path", r.URL.Path,
		"socket", socketPath,
		"allowed", allowed,
		"reason", reason,
	)

	if !allowed {
		http.Error(w, "Access denied by ACL: "+reason, http.StatusForbidden)
		return
	}

	// Create a Unix socket connection to Docker
	conn, err := net.Dial("unix", h.dockerSocket)
	if err != nil {
		log.Error("Failed to connect to Docker socket", "error", err)
		http.Error(w, "Failed to connect to Docker socket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Create a new HTTP client connection
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	// Copy the original request
	outreq := new(http.Request)
	*outreq = *r

	// Set the URL for the Docker API
	outreq.URL.Scheme = "http"
	outreq.URL.Host = "unix"

	// Send the request
	resp, err := client.Do(outreq)
	if err != nil {
		log.Error("Failed to send request to Docker", "error", err)
		http.Error(w, "Failed to send request to Docker: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy the response headers
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	// Copy the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	io.Copy(w, resp.Body)

	// Log the response time
	log.Debug("Proxy response",
		"method", r.Method,
		"path", r.URL.Path,
		"socket", socketPath,
		"duration", time.Since(start),
	)
}

// checkACLs checks if a request is allowed by the ACLs
func (h *ProxyHandler) checkACLs(r *http.Request, socketConfig *config.SocketConfig) (bool, string) {
	// If there's no config, allow all requests
	if socketConfig == nil {
		return true, ""
	}

	// If there are no ACLs, deny by default
	if len(socketConfig.Rules.ACLs) == 0 {
		return false, "no ACLs defined"
	}

	// Check each ACL rule
	for _, rule := range socketConfig.Rules.ACLs {
		// Check if the rule matches the request
		if h.ruleMatches(r, rule.Match) {
			if rule.Action == "allow" {
				return true, ""
			} else {
				return false, "method not allowed"
			}
		}
	}

	// If no rule matches, deny by default
	return false, "No matching allow rules"
}

// ruleMatches checks if a request matches an ACL rule
func (h *ProxyHandler) ruleMatches(r *http.Request, match config.Match) bool {
	// Check path match
	if match.Path != "" && !strings.HasPrefix(r.URL.Path, match.Path) {
		return false
	}

	// Check method match
	if match.Method != "" && r.Method != match.Method {
		return false
	}

	return true
}
