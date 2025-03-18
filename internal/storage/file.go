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
	// Create a config file next to each socket with .sock.config extension
	return socketPath + ".config"
}

func (s *FileStore) SaveConfig(socketPath string, cfg *config.SocketConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath(socketPath), data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (s *FileStore) LoadConfig(socketPath string) (*config.SocketConfig, error) {
	data, err := os.ReadFile(s.configPath(socketPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.SocketConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (s *FileStore) LoadExistingConfigs() (map[string]*config.SocketConfig, error) {
	log := logging.GetLogger()
	configs := make(map[string]*config.SocketConfig)

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	log.Debug("Scanning directory for configs", "dir", s.baseDir)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sock.config") {
			continue
		}

		log.Debug("Found config file", "name", name)

		// Get the socket path by removing .config suffix
		socketPath := strings.TrimSuffix(filepath.Join(s.baseDir, name), ".config")
		cfg, err := s.LoadConfig(socketPath)
		if err != nil {
			log.Error("Failed to load config", "path", socketPath, "error", err)
			continue
		}
		if cfg != nil {
			log.Debug("Loaded config successfully", "socket", socketPath)
			configs[socketPath] = cfg
		}
	}

	log.Debug("Completed loading configs", "count", len(configs))
	return configs, nil
}

func (s *FileStore) DeleteConfig(socketPath string) error {
	err := os.Remove(s.configPath(socketPath))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}
	return nil
}
