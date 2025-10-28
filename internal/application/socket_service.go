package application

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/logging"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// SocketService defines the interface for socket management operations
type SocketService interface {
	CreateSocket(ctx context.Context, config domain.SocketConfig) (domain.Socket, error)
	DeleteSocket(ctx context.Context, socketName string) error
	ListSockets(ctx context.Context) ([]string, error)
	DescribeSocket(ctx context.Context, socketName string) (domain.SocketConfig, error)
	CleanSockets(ctx context.Context) error
}

// SocketRepository defines the interface for socket data persistence
type SocketRepository interface {
	Save(ctx context.Context, name string, config domain.SocketConfig) error
	Load(ctx context.Context, name string) (domain.SocketConfig, error)
	Delete(ctx context.Context, name string) error
	List(ctx context.Context) (map[string]domain.SocketConfig, error)
}

// socketService implements SocketService
type socketService struct {
	repo    SocketRepository
	manager SocketManager
}

// NewSocketService creates a new SocketService
func NewSocketService(repo SocketRepository, manager SocketManager) SocketService {
	return &socketService{
		repo:    repo,
		manager: manager,
	}
}

// CreateSocket creates a new socket with the given configuration
func (s *socketService) CreateSocket(ctx context.Context, config domain.SocketConfig) (domain.Socket, error) {
	// Validate and set socket name
	socketName := config.Name
	if socketName == "" {
		socketName = fmt.Sprintf("docker-proxy-%s", uuid.New().String())
	}

	// Validate socket name (no slashes, reasonable length)
	if strings.Contains(socketName, "/") || strings.Contains(socketName, "\\") {
		return domain.Socket{}, fmt.Errorf("socket name cannot contain path separators: %s", socketName)
	}
	if len(socketName) > 100 {
		return domain.Socket{}, fmt.Errorf("socket name too long (max 100 chars): %s", socketName)
	}

	// Determine socket path
	var socketPath string
	if config.ListenAddress != "" {
		// Use provided path
		socketPath = config.ListenAddress
	} else {
		// Generate path from name
		socketPath = filepath.Join(s.manager.GetSocketDir(), socketName+".sock")
	}

	// Update config with proper name and path
	config.Name = socketName
	config.ListenAddress = socketPath

	// Create the socket
	socket := domain.Socket{
		Path:   socketPath,
		Config: config,
	}

	// Save configuration by name
	if err := s.repo.Save(ctx, socketName, config); err != nil {
		return domain.Socket{}, fmt.Errorf("failed to save socket configuration: %w", err)
	}

	// Register with manager by name
	if err := s.manager.CreateSocket(socketName, config); err != nil {
		// Clean up repository on manager error
		if deleteErr := s.repo.Delete(ctx, socketName); deleteErr != nil {
			logging.GetLogger().Error("failed to clean up repository after manager error", "error", deleteErr)
		}
		return domain.Socket{}, fmt.Errorf("failed to create socket: %w", err)
	}

	logging.GetLogger().Info("Created socket", "name", socketName, "path", socketPath)
	return socket, nil
}

// DeleteSocket deletes a socket
func (s *socketService) DeleteSocket(ctx context.Context, socketName string) error {
	// Check if socket exists
	config, exists := s.manager.GetSocket(socketName)
	if !exists {
		return fmt.Errorf("socket not found: %s", socketName)
	}

	// Get the socket path from the config
	socketPath := config.ListenAddress

	// Remove from manager
	if err := s.manager.DeleteSocket(socketName); err != nil {
		return fmt.Errorf("failed to delete socket from manager: %w", err)
	}

	// Remove from repository
	if err := s.repo.Delete(ctx, socketName); err != nil {
		// Log error but don't fail the operation
		logging.GetLogger().Error("Failed to delete socket from repository", "error", err, "name", socketName)
	}

	// Remove socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		logging.GetLogger().Error("Failed to remove socket file", "error", err, "path", socketPath)
	}

	logging.GetLogger().Info("Deleted socket", "name", socketName, "path", socketPath)
	return nil
}

// ListSockets lists all available sockets
func (s *socketService) ListSockets(ctx context.Context) ([]string, error) {
	socketNames := s.manager.ListSockets()
	return socketNames, nil
}

// DescribeSocket describes a socket's configuration
func (s *socketService) DescribeSocket(ctx context.Context, socketName string) (domain.SocketConfig, error) {
	config, exists := s.manager.GetSocket(socketName)
	if !exists {
		return domain.SocketConfig{}, fmt.Errorf("socket not found: %s", socketName)
	}

	return config, nil
}

// CleanSockets removes all sockets
func (s *socketService) CleanSockets(ctx context.Context) error {
	socketNames := s.manager.ListSockets()

	var errors []error
	for _, socketName := range socketNames {
		if err := s.DeleteSocket(ctx, socketName); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete socket %s: %w", socketName, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to clean some sockets: %v", errors)
	}

	logging.GetLogger().Info("Cleaned all sockets", "count", len(socketNames))
	return nil
}

// inMemorySocketManager implements SocketManager for testing
type inMemorySocketManager struct {
	sockets map[string]domain.SocketConfig
	mu      sync.RWMutex
}

// NewInMemorySocketManager creates a new in-memory socket manager
func NewInMemorySocketManager() SocketManager {
	return &inMemorySocketManager{
		sockets: make(map[string]domain.SocketConfig),
	}
}

// CreateSocket creates a socket in memory
func (m *inMemorySocketManager) CreateSocket(name string, config domain.SocketConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sockets[name] = config
	return nil
}

// DeleteSocket deletes a socket from memory
func (m *inMemorySocketManager) DeleteSocket(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sockets, name)
	return nil
}

// ListSockets lists all socket names
func (m *inMemorySocketManager) ListSockets() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.sockets))
	for name := range m.sockets {
		names = append(names, name)
	}
	return names
}

// GetSocket gets a socket configuration
func (m *inMemorySocketManager) GetSocket(name string) (domain.SocketConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, exists := m.sockets[name]
	return config, exists
}

// GetSocketDir returns the socket directory (empty for in-memory)
func (m *inMemorySocketManager) GetSocketDir() string {
	return ""
}
