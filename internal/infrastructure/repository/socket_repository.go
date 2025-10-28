package repository

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"docker-socket-proxy/internal/logging"
)

// SocketRepository defines the interface for socket configuration persistence
type SocketRepository interface {
	Save(ctx context.Context, name string, config domain.SocketConfig) error
	Load(ctx context.Context, name string) (domain.SocketConfig, error)
	Delete(ctx context.Context, name string) error
	List(ctx context.Context) (map[string]domain.SocketConfig, error)
}

// InMemorySocketRepository is an in-memory implementation for testing
type InMemorySocketRepository struct {
	configs map[string]domain.SocketConfig
	mu      sync.RWMutex
}

// NewInMemorySocketRepository creates a new in-memory repository
func NewInMemorySocketRepository() *InMemorySocketRepository {
	return &InMemorySocketRepository{
		configs: make(map[string]domain.SocketConfig),
	}
}

// Save stores a socket configuration in memory
func (r *InMemorySocketRepository) Save(ctx context.Context, name string, config domain.SocketConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[name] = config
	return nil
}

// Load retrieves a socket configuration by name
func (r *InMemorySocketRepository) Load(ctx context.Context, name string) (domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, exists := r.configs[name]
	if !exists {
		return domain.SocketConfig{}, fmt.Errorf("socket config not found: %s", name)
	}
	return config, nil
}

// List retrieves all socket configurations
func (r *InMemorySocketRepository) List(ctx context.Context) (map[string]domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	configs := make(map[string]domain.SocketConfig)
	for k, v := range r.configs {
		configs[k] = v
	}
	return configs, nil
}

// Delete removes a socket configuration by name
func (r *InMemorySocketRepository) Delete(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.configs, name)
	return nil
}

// FileSocketRepository is a file-based implementation for production
type FileSocketRepository struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileSocketRepository creates a new file-based repository
func NewFileSocketRepository(baseDir string) *FileSocketRepository {
	return &FileSocketRepository{
		baseDir: baseDir,
	}
}

// Save stores a socket configuration to a file
func (r *FileSocketRepository) Save(ctx context.Context, name string, config domain.SocketConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := r.getFilename(name)

	// Create the directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal the config to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	log := logging.GetLogger()
	log.Debug("Saved socket config", "name", name, "filename", filename)

	return nil
}

// Load retrieves a socket configuration by name from file
func (r *FileSocketRepository) Load(ctx context.Context, name string) (domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filename := r.getFilename(name)

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
	err = json.Unmarshal(data, &config)
	if err != nil {
		return domain.SocketConfig{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Don't override the name and ListenAddress - they should be preserved from the config
	log := logging.GetLogger()
	log.Debug("Loaded socket config", "name", name, "filename", filename)

	return config, nil
}

// List retrieves all socket configurations from files
func (r *FileSocketRepository) List(ctx context.Context) (map[string]domain.SocketConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get all files in the base directory
	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	configs := make(map[string]domain.SocketConfig)
	log := logging.GetLogger()

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

		// Get the socket name from the filename (remove .json extension)
		socketName := strings.TrimSuffix(file.Name(), ".json")

		// Load the config
		config, err := r.Load(ctx, socketName)
		if err != nil {
			log.Error("Failed to load config", "name", socketName, "error", err)
			continue
		}

		configs[socketName] = config
	}

	return configs, nil
}

// Delete removes a socket configuration file
func (r *FileSocketRepository) Delete(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filename := r.getFilename(name)
	err := os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}

	log := logging.GetLogger()
	log.Debug("Deleted socket config", "name", name, "filename", filename)

	return nil
}

// getFilename returns the filename for a socket name
func (r *FileSocketRepository) getFilename(socketName string) string {
	// Replace slashes with underscores (in case the name itself contains slashes)
	filename := strings.ReplaceAll(socketName, "/", "_")

	// Add the .json extension
	filename = filename + ".json"

	// Join with the base directory
	return filepath.Join(r.baseDir, filename)
}
