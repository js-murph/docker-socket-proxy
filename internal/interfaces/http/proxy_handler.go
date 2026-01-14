package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"docker-socket-proxy/internal/application"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/logging"
)

// ProxyHandler handles HTTP requests for proxying to Docker socket
type ProxyHandler struct {
	proxyService application.ProxyService
	logger       *slog.Logger
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(proxyService application.ProxyService) *ProxyHandler {
	return &ProxyHandler{
		proxyService: proxyService,
		logger:       logging.GetLogger(),
	}
}

// ServeHTTP handles HTTP requests to the proxy server
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get the socket name from the server name or context
	socketName := h.getSocketName(r)
	if socketName == "" {
		h.logger.Error("No socket name found in request")
		http.Error(w, "No socket name found", http.StatusBadRequest)
		return
	}

	// Process the request using the proxy service
	response, err := h.proxyService.ProcessRequest(r.Context(), r, socketName)
	if err != nil {
		h.logger.Error("Failed to process request", "error", err)
		http.Error(w, "Failed to process request", http.StatusInternalServerError)
		return
	}

	// Copy response headers
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(response.StatusCode)

	// Copy response body
	if response.Body != nil {
		defer func() {
			if err := response.Body.Close(); err != nil {
				logging.GetLogger().Error("failed to close response body", "error", err)
			}
		}()
		if _, err := io.Copy(w, response.Body); err != nil {
			logging.GetLogger().Error("failed to copy response body", "error", err)
		}
	}
}

// getSocketName extracts the socket name from the request
func (h *ProxyHandler) getSocketName(r *http.Request) string {
	// Try to get from context first
	if socketName, ok := r.Context().Value(domain.SocketContextKey).(string); ok {
		return socketName
	}

	// Try to get from header
	if socketName := r.Header.Get("Socket-Name"); socketName != "" {
		return socketName
	}

	// Try to get from query parameter
	if socketName := r.URL.Query().Get("socket"); socketName != "" {
		return socketName
	}

	// Try to get from server name (for virtual hosts)
	if r.TLS != nil && r.TLS.ServerName != "" {
		return r.TLS.ServerName
	}

	return ""
}

// ServeHTTPWithSocket forwards the request to the Docker socket with explicit socket name
func (h *ProxyHandler) ServeHTTPWithSocket(w http.ResponseWriter, r *http.Request, socketName string) {
	// Add socket name to context
	ctx := context.WithValue(r.Context(), domain.SocketContextKey, socketName)
	r = r.WithContext(ctx)

	// Process the request
	h.ServeHTTP(w, r)
}
