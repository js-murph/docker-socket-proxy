package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
)

type FileStore struct {
	baseDir string
}

func NewFileStore(managementSocket string) *FileStore {
	// Store configs in the same directory as the management socket
	baseDir := filepath.Dir(managementSocket)
	return &FileStore{
		baseDir: baseDir,
	}
}

func (s *FileStore) configPath(socketPath string) string {
	// Create a config file next to each socket with .config extension
	return socketPath + ".config"
}

// SaveConfig saves a socket configuration
func (s *FileStore) SaveConfig(socketPath string, cfg *config.SocketConfig) error {
	log := logging.GetLogger()

	// Get the filename for the socket
	filename := s.getFilename(socketPath)

	// Create the directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal the config to JSON
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Log the marshaled JSON for debugging
	log.Debug("Marshaled config to JSON", "json", string(data))

	// Write the file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// LoadConfig loads a socket configuration
func (s *FileStore) LoadConfig(socketPath string) (*config.SocketConfig, error) {
	log := logging.GetLogger()

	// Get the filename for the socket
	filename := s.getFilename(socketPath)

	// Check if the file exists
	if _, err := os.Stat(filename); err != nil {
		return nil, fmt.Errorf("config file not found: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Log the raw JSON for debugging
	log.Debug("Raw JSON config data", "filename", filename, "data", string(data))

	// Parse the JSON
	var socketConfig config.SocketConfig
	if err := json.Unmarshal(data, &socketConfig); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config file: %w", err)
	}

	// Log the loaded config
	log.Debug("Loaded config from JSON", "filename", filename, "num_acls", len(socketConfig.Rules.ACLs))

	// Validate the config
	if err := config.ValidateConfig(&socketConfig); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &socketConfig, nil
}

// LoadExistingConfigs loads all existing socket configurations
func (s *FileStore) LoadExistingConfigs() (map[string]*config.SocketConfig, error) {
	log := logging.GetLogger()

	// Get all files in the base directory
	files, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Create a map to store the configs
	configs := make(map[string]*config.SocketConfig)

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
		config, err := s.LoadConfig(socketPath)
		if err != nil {
			log.Error("Failed to load config", "path", socketPath, "error", err)
			continue
		}

		// Add the config to the map
		configs[socketPath] = config
	}

	return configs, nil
}

// getFilename returns the filename for a socket path
func (s *FileStore) getFilename(socketPath string) string {
	// Extract just the socket name from the path
	socketName := filepath.Base(socketPath)

	// Replace slashes with underscores (in case the name itself contains slashes)
	filename := strings.Replace(socketName, "/", "_", -1)

	// Add the .json extension
	filename = filename + ".json"

	// Join with the base directory
	return filepath.Join(s.baseDir, filename)
}

func (s *FileStore) DeleteConfig(socketPath string) error {
	filename := s.getFilename(socketPath)
	err := os.Remove(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}
	return nil
}
