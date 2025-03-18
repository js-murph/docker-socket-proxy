package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"

	"gopkg.in/yaml.v3"
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

func (s *FileStore) SaveConfig(socketPath string, cfg *config.SocketConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := s.configPath(socketPath)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (s *FileStore) LoadConfig(socketPath string) (*config.SocketConfig, error) {
	configPath := s.configPath(socketPath)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.SocketConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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
		if entry.IsDir() {
			continue
		}

		// Only look for .config files
		if !strings.HasSuffix(name, ".config") {
			continue
		}

		// Get the socket path by removing .config suffix
		socketPath := filepath.Join(s.baseDir, strings.TrimSuffix(name, ".config"))
		log.Debug("Found config file", "socket", socketPath)

		cfg, err := s.LoadConfig(socketPath)
		if err != nil {
			log.Error("Failed to load config", "path", socketPath, "error", err)
			continue
		}
		if cfg != nil {
			configs[socketPath] = cfg
		}
	}

	return configs, nil
}

func (s *FileStore) DeleteConfig(socketPath string) error {
	err := os.Remove(s.configPath(socketPath))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}
	return nil
}
