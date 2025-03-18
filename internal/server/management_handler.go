package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"

	"github.com/google/uuid"
)

type ManagementHandler struct {
	dockerSocket  string
	socketConfigs map[string]*config.SocketConfig
	configMu      *sync.RWMutex
	proxyHandler  *ProxyHandler
	servers       map[string]*http.Server
	serverMu      sync.RWMutex
	store         *storage.FileStore
}

func NewManagementHandler(dockerSocket string, configs map[string]*config.SocketConfig, mu *sync.RWMutex, store *storage.FileStore) *ManagementHandler {
	return &ManagementHandler{
		dockerSocket:  dockerSocket,
		socketConfigs: configs,
		configMu:      mu,
		proxyHandler:  NewProxyHandler(dockerSocket, configs, mu),
		servers:       make(map[string]*http.Server),
		store:         store,
	}
}

func (h *ManagementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/create-socket" && r.Method == "POST":
		h.handleCreateSocket(w, r)
	case r.URL.Path == "/delete-socket" && r.Method == "DELETE":
		h.handleDeleteSocket(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// generateSocketPath creates a unique socket path in the system's temp directory
func generateSocketPath(socketDir string) string {
	return filepath.Join(socketDir, fmt.Sprintf("docker-proxy-%s.sock", uuid.New().String()))
}

func (h *ManagementHandler) validateAndDecodeConfig(r *http.Request) (*config.SocketConfig, error) {
	var socketConfig *config.SocketConfig
	if r.Body == nil {
		return nil, fmt.Errorf("empty request body")
	}

	if err := json.NewDecoder(r.Body).Decode(&socketConfig); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty configuration provided")
		}
		return nil, fmt.Errorf("invalid configuration format: %v", err)
	}

	if err := config.ValidateConfig(socketConfig); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return socketConfig, nil
}

func (h *ManagementHandler) handleCreateSocket(w http.ResponseWriter, r *http.Request) {
	socketConfig, err := h.validateAndDecodeConfig(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s, ok := r.Context().Value(serverContextKey).(*Server)
	if !ok {
		http.Error(w, "Server context not found", http.StatusInternalServerError)
		return
	}

	socketPath := generateSocketPath(s.socketDir)
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("failed to remove existing socket: %v", err), http.StatusInternalServerError)
		return
	}

	// Save config before creating socket
	if err := h.store.SaveConfig(socketPath, socketConfig); err != nil {
		http.Error(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		h.store.DeleteConfig(socketPath) // Cleanup config if socket creation fails
		http.Error(w, fmt.Sprintf("failed to create socket listener: %v", err), http.StatusInternalServerError)
		return
	}

	h.configMu.Lock()
	h.socketConfigs[socketPath] = socketConfig
	h.configMu.Unlock()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.proxyHandler.ServeHTTP(w, r, socketPath)
		}),
	}

	h.serverMu.Lock()
	h.servers[socketPath] = server
	h.serverMu.Unlock()

	// Track the created socket
	if s, ok := r.Context().Value(serverContextKey).(*Server); ok {
		s.TrackSocket(socketPath)
	}

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			log := logging.GetLogger()
			log.Error("Proxy server error", "error", err, "socket", socketPath)
		}
	}()

	w.Write([]byte(socketPath))
}

func (h *ManagementHandler) handleDeleteSocket(w http.ResponseWriter, r *http.Request) {
	socketPath := r.Header.Get("Socket-Path")
	if socketPath == "" {
		http.Error(w, "Socket-Path header is required", http.StatusBadRequest)
		return
	}

	h.serverMu.Lock()
	if server, exists := h.servers[socketPath]; exists {
		server.Shutdown(context.Background())
		delete(h.servers, socketPath)
	}
	h.serverMu.Unlock()

	h.configMu.Lock()
	delete(h.socketConfigs, socketPath)
	h.configMu.Unlock()

	if err := h.store.DeleteConfig(socketPath); err != nil {
		log := logging.GetLogger()
		log.Error("Failed to delete config file", "error", err)
	}

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("failed to remove socket: %v", err), http.StatusInternalServerError)
		return
	}

	if s, ok := r.Context().Value(serverContextKey).(*Server); ok {
		s.UntrackSocket(socketPath)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *ManagementHandler) Cleanup() {
	h.serverMu.Lock()
	defer h.serverMu.Unlock()

	for path, server := range h.servers {
		server.Close()
		os.Remove(path)
		delete(h.servers, path)
	}
}
