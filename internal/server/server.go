package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
				s.configMu.RLock()
				socketConfig, ok := s.socketConfigs[socketPath]
				s.configMu.RUnlock()

				if ok && socketConfig != nil {
					// Serve the request
					proxyHandler.ServeHTTPWithSocket(w, r, socketPath)
				}
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
