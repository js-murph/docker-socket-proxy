package application

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileSocketRepository implements SocketRepository using file storage
type FileSocketRepository struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileSocketRepository creates a new file-based socket repository
func NewFileSocketRepository(baseDir string) SocketRepository {
	return &FileSocketRepository{
		baseDir: baseDir,
	}
}

// Save saves a socket configuration to file
func (r *FileSocketRepository) Save(ctx context.Context, path string, config domain.SocketConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get the filename for the socket
	filename := r.getFilename(path)

	// Create the directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal the config to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Load loads a socket configuration from file
func (r *FileSocketRepository) Load(ctx context.Context, path string) (domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the filename for the socket
	filename := r.getFilename(path)

	// Check if the file exists
	if _, err := os.Stat(filename); err != nil {
		return domain.SocketConfig{}, fmt.Errorf("config file not found: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return domain.SocketConfig{}, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the JSON
	var config domain.SocketConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return domain.SocketConfig{}, fmt.Errorf("failed to parse JSON config file: %w", err)
	}

	return config, nil
}

// Delete deletes a socket configuration file
func (r *FileSocketRepository) Delete(ctx context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := r.getFilename(path)
	err := os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}
	return nil
}

// List lists all socket configurations
func (r *FileSocketRepository) List(ctx context.Context) (map[string]domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get all files in the base directory
	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Create a map to store the configs
	configs := make(map[string]domain.SocketConfig)

	// Load each config file
	for _, file := range files {
		// Skip directories
		if file.IsDir() {
			continue
		}

		// Skip files that don't have the .json extension
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		// Get the socket path from the filename
		socketPath := strings.TrimSuffix(file.Name(), ".json")
		socketPath = strings.ReplaceAll(socketPath, "_", "/")

		// If the socket path doesn't end with .sock, add it
		if !strings.HasSuffix(socketPath, ".sock") {
			socketPath = socketPath + ".sock"
		}

		// If the socket path doesn't start with /, add it
		if !strings.HasPrefix(socketPath, "/") {
			socketPath = "/" + socketPath
		}

		// Load the config
		config, err := r.Load(ctx, socketPath)
		if err != nil {
			// Log error but continue
			continue
		}

		// Add the config to the map
		configs[socketPath] = config
	}

	return configs, nil
}

// getFilename returns the filename for a socket path
func (r *FileSocketRepository) getFilename(socketPath string) string {
	// Extract just the socket name from the path
	socketName := filepath.Base(socketPath)

	// Replace slashes with underscores (in case the name itself contains slashes)
	filename := strings.ReplaceAll(socketName, "/", "_")

	// Add the .json extension
	filename = filename + ".json"

	// Join with the base directory
	return filepath.Join(r.baseDir, filename)
}

// InMemorySocketRepository implements SocketRepository using in-memory storage
type InMemorySocketRepository struct {
	configs map[string]domain.SocketConfig
	mu      sync.RWMutex
}

// NewInMemorySocketRepository creates a new in-memory socket repository
func NewInMemorySocketRepository() SocketRepository {
	return &InMemorySocketRepository{
		configs: make(map[string]domain.SocketConfig),
	}
}

// Save saves a socket configuration in memory
func (r *InMemorySocketRepository) Save(ctx context.Context, path string, config domain.SocketConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[path] = config
	return nil
}

// Load loads a socket configuration from memory
func (r *InMemorySocketRepository) Load(ctx context.Context, path string) (domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, exists := r.configs[path]
	if !exists {
		return domain.SocketConfig{}, fmt.Errorf("config not found: %s", path)
	}
	return config, nil
}

// Delete deletes a socket configuration from memory
func (r *InMemorySocketRepository) Delete(ctx context.Context, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.configs, path)
	return nil
}

// List lists all socket configurations
func (r *InMemorySocketRepository) List(ctx context.Context) (map[string]domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a copy of the configs map
	configs := make(map[string]domain.SocketConfig)
	for k, v := range r.configs {
		configs[k] = v
	}

	return configs, nil
}
