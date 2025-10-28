package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docker-socket-proxy/internal/application"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/infrastructure/repository"
	"docker-socket-proxy/internal/logging"
)

// Server represents the HTTP server
type Server struct {
	managementHandler *ManagementHandler
	proxyHandler      *ProxyHandler
	server            *http.Server
	logger            *slog.Logger
}

// NewServer creates a new HTTP server
func NewServer(
	socketService application.SocketService,
	proxyService application.ProxyService,
	addr string,
) *Server {
	// Create handlers
	managementHandler := NewManagementHandler(socketService)
	proxyHandler := NewProxyHandler(proxyService)

	// Create mux
	mux := http.NewServeMux()

	// Register management routes
	mux.HandleFunc("/socket/create", managementHandler.CreateSocketHandler)
	mux.HandleFunc("/socket/list", managementHandler.ListSocketsHandler)
	mux.HandleFunc("/socket/describe", managementHandler.DescribeSocketHandler)
	mux.HandleFunc("/socket/delete", managementHandler.DeleteSocketHandler)
	mux.HandleFunc("/socket/clean", managementHandler.CleanSocketsHandler)
	mux.HandleFunc("/health", managementHandler.HealthHandler)

	// Register proxy routes
	mux.HandleFunc("/", proxyHandler.ServeHTTP)

	// Create HTTP server
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return &Server{
		managementHandler: managementHandler,
		proxyHandler:      proxyHandler,
		server:            server,
		logger:            logging.GetLogger(),
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("Starting HTTP server", "addr", s.server.Addr)

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.Info("Shutting down HTTP server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
		return err
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

// Stop stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping HTTP server...")
	return s.server.Shutdown(ctx)
}

// UnixServer represents a Unix domain socket server
type UnixServer struct {
	managementHandler *ManagementHandler
	proxyHandler      *ProxyHandler
	server            *http.Server
	logger            *slog.Logger
}

// NewUnixServer creates a new Unix domain socket server
func NewUnixServer(
	socketService application.SocketService,
	proxyService application.ProxyService,
	socketPath string,
) *UnixServer {
	// Create handlers
	managementHandler := NewManagementHandler(socketService)
	proxyHandler := NewProxyHandler(proxyService)

	// Create mux
	mux := http.NewServeMux()

	// Register management routes
	mux.HandleFunc("/socket/create", managementHandler.CreateSocketHandler)
	mux.HandleFunc("/socket/list", managementHandler.ListSocketsHandler)
	mux.HandleFunc("/socket/describe", managementHandler.DescribeSocketHandler)
	mux.HandleFunc("/socket/delete", managementHandler.DeleteSocketHandler)
	mux.HandleFunc("/socket/clean", managementHandler.CleanSocketsHandler)
	mux.HandleFunc("/health", managementHandler.HealthHandler)

	// Register proxy routes
	mux.HandleFunc("/", proxyHandler.ServeHTTP)

	// Create HTTP server
	server := &http.Server{
		Handler: mux,
	}

	return &UnixServer{
		managementHandler: managementHandler,
		proxyHandler:      proxyHandler,
		server:            server,
		logger:            logging.GetLogger(),
	}
}

// Start starts the Unix domain socket server
func (s *UnixServer) Start(socketPath string) error {
	s.logger.Info("Starting Unix domain socket server", "path", socketPath)

	// Remove existing socket file
	if err := os.RemoveAll(socketPath); err != nil {
		s.logger.Warn("Failed to remove existing socket file", "path", socketPath, "error", err)
	}

	// Create listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix domain socket listener: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		s.logger.Warn("Failed to set socket permissions", "path", socketPath, "error", err)
	}

	// Start server in goroutine
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Unix domain socket server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.Info("Shutting down Unix domain socket server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
		return err
	}

	// Clean up socket file
	if err := os.RemoveAll(socketPath); err != nil {
		s.logger.Warn("Failed to remove socket file", "path", socketPath, "error", err)
	}

	s.logger.Info("Unix domain socket server stopped")
	return nil
}

// Stop stops the Unix domain socket server
func (s *UnixServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Unix domain socket server...")
	return s.server.Shutdown(ctx)
}

// ServerFactory creates servers with proper dependency injection
type ServerFactory struct {
	socketService application.SocketService
	proxyService  application.ProxyService
}

// NewServerFactory creates a new server factory
func NewServerFactory(socketService application.SocketService, proxyService application.ProxyService) *ServerFactory {
	return &ServerFactory{
		socketService: socketService,
		proxyService:  proxyService,
	}
}

// CreateHTTPServer creates an HTTP server
func (f *ServerFactory) CreateHTTPServer(addr string) *Server {
	return NewServer(f.socketService, f.proxyService, addr)
}

// CreateUnixServer creates a Unix domain socket server
func (f *ServerFactory) CreateUnixServer(socketPath string) *UnixServer {
	return NewUnixServer(f.socketService, f.proxyService, socketPath)
}

// CreateServerFromConfig creates a server based on configuration
func CreateServerFromConfig(config ServerConfig) (*Server, error) {
	// Create repository
	var repo application.SocketRepository
	if config.UseFileStorage {
		repo = repository.NewFileSocketRepository(config.StorageDir)
	} else {
		repo = repository.NewInMemorySocketRepository()
	}

	// Create mock dependencies
	manager := &MockSocketManager{}

	// Create services
	socketService := application.NewSocketService(repo, manager)
	proxyService := application.NewProxyService(&MockDockerClient{}, manager)

	// Create server factory
	factory := NewServerFactory(socketService, proxyService)

	// Create server
	server := factory.CreateHTTPServer(config.Addr)

	return server, nil
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Addr           string
	UseFileStorage bool
	StorageDir     string
}

// MockSocketManager is a mock implementation of SocketManager for testing
type MockSocketManager struct{}

func (m *MockSocketManager) CreateSocket(name string, config domain.SocketConfig) error {
	return nil
}

func (m *MockSocketManager) DeleteSocket(name string) error {
	return nil
}

func (m *MockSocketManager) ListSockets() []string {
	return []string{}
}

func (m *MockSocketManager) GetSocket(name string) (domain.SocketConfig, bool) {
	return domain.SocketConfig{}, false
}

func (m *MockSocketManager) GetSocketDir() string {
	return "/tmp/sockets"
}

// MockDockerClient is a mock implementation of DockerClient for testing
type MockDockerClient struct{}

func (m *MockDockerClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}
