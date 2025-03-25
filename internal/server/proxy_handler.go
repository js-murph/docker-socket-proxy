package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
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
