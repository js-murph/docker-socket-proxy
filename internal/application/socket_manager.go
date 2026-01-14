package application

import (
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/logging"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SocketManager manages socket lifecycle
type SocketManager interface {
	CreateSocket(name string, config domain.SocketConfig) error
	DeleteSocket(name string) error
	ListSockets() []string
	GetSocket(name string) (domain.SocketConfig, bool)
	GetSocketDir() string
}

// socketManager implements SocketManager
type socketManager struct {
	socketDir string
	mu        sync.RWMutex
	sockets   map[string]domain.SocketConfig
}

// NewSocketManager creates a new SocketManager
func NewSocketManager(socketDir string) SocketManager {
	return &socketManager{
		socketDir: socketDir,
		sockets:   make(map[string]domain.SocketConfig),
	}
}

// CreateSocket creates a new socket
func (m *socketManager) CreateSocket(name string, config domain.SocketConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store socket configuration by name
	m.sockets[name] = config

	// Create socket file using the path from config
	socketPath := config.ListenAddress
	if err := m.createSocketFile(socketPath); err != nil {
		delete(m.sockets, name)
		return err
	}

	logging.GetLogger().Info("Socket created", "name", name, "path", socketPath)
	return nil
}

// DeleteSocket deletes a socket
func (m *socketManager) DeleteSocket(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the socket path from config before removing
	config, exists := m.sockets[name]
	if !exists {
		return fmt.Errorf("socket not found: %s", name)
	}
	socketPath := config.ListenAddress

	// Remove from memory
	delete(m.sockets, name)

	// Remove socket file
	if err := m.removeSocketFile(socketPath); err != nil {
		logging.GetLogger().Warn("Failed to remove socket file", "path", socketPath, "error", err)
	}

	logging.GetLogger().Info("Socket deleted", "name", name, "path", socketPath)
	return nil
}

// ListSockets lists all socket names
func (m *socketManager) ListSockets() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.sockets))
	for name := range m.sockets {
		names = append(names, name)
	}
	return names
}

// GetSocket gets a socket configuration
func (m *socketManager) GetSocket(name string) (domain.SocketConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, exists := m.sockets[name]
	return config, exists
}

// GetSocketDir returns the socket directory
func (m *socketManager) GetSocketDir() string {
	return m.socketDir
}

// createSocketFile creates the actual socket file
func (m *socketManager) createSocketFile(socketPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create socket file
	file, err := os.Create(socketPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logging.GetLogger().Error("failed to close file", "error", err)
		}
	}()

	// Set permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		return err
	}

	return nil
}

// removeSocketFile removes the socket file
func (m *socketManager) removeSocketFile(socketPath string) error {
	return os.Remove(socketPath)
}
