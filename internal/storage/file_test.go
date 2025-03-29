package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
)

func TestFileStore(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("/tmp", "filestore-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

	// Create test socket paths with unique names
	testSocketName := "test.sock"
	testSocketPath := filepath.Join(tempDir, testSocketName)

	anotherSocketName := "another.sock"
	anotherSocketPath := filepath.Join(tempDir, anotherSocketName)

	// Create a test config with valid rules
	testConfig := &config.SocketConfig{
		Config: config.ConfigSet{
			PropagateSocket: "/var/run/docker.sock",
		},
		Rules: []config.Rule{
			{
				Match: config.Match{
					Path:   "/.*",
					Method: ".*",
				},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
		},
	}

	// Create a file store
	store := NewFileStore(tempDir)

	// Test saving a config
	t.Run("save_config", func(t *testing.T) {
		err := store.SaveConfig(testSocketPath, testConfig)
		if err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}

		// Check if the file exists
		filename := store.getFilename(testSocketPath)
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Errorf("SaveConfig() did not create file %s", filename)
		}
	})

	// Test loading a config
	t.Run("load_config", func(t *testing.T) {
		cfg, err := store.LoadConfig(testSocketPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		// Check if the config matches
		if cfg.Config.PropagateSocket != testConfig.Config.PropagateSocket {
			t.Errorf("LoadConfig() got = %v, want %v", cfg.Config.PropagateSocket, testConfig.Config.PropagateSocket)
		}

		if len(cfg.Rules) != len(testConfig.Rules) {
			t.Errorf("LoadConfig() got %d Rules, want %d", len(cfg.Rules), len(testConfig.Rules))
		}
	})

	// Test loading existing configs
	t.Run("load_existing_configs", func(t *testing.T) {
		// Create another test socket config
		err := store.SaveConfig(anotherSocketPath, testConfig)
		if err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}

		// Load existing configs
		configs, err := store.LoadExistingConfigs()
		if err != nil {
			t.Fatalf("LoadExistingConfigs() error = %v", err)
		}

		// Check if both configs are loaded
		if len(configs) < 2 {
			t.Errorf("LoadExistingConfigs() got %d configs, want at least 2", len(configs))
		}

		// Check if the test socket config is loaded
		testBaseName := filepath.Base(testSocketPath)
		anotherBaseName := filepath.Base(anotherSocketPath)

		testFound := false
		anotherFound := false

		for path := range configs {
			if strings.Contains(path, testBaseName) {
				testFound = true
			}
			if strings.Contains(path, anotherBaseName) {
				anotherFound = true
			}
		}

		if !testFound {
			t.Errorf("LoadExistingConfigs() missing test socket config: %s", testSocketPath)
		}

		if !anotherFound {
			t.Errorf("LoadExistingConfigs() missing another socket config: %s", anotherSocketPath)
		}
	})

	// Test deleting a config
	t.Run("delete_config", func(t *testing.T) {
		err := store.DeleteConfig(testSocketPath)
		if err != nil {
			t.Fatalf("DeleteConfig() error = %v", err)
		}

		// Check if the file is deleted
		filename := store.getFilename(testSocketPath)
		if _, err := os.Stat(filename); !os.IsNotExist(err) {
			t.Errorf("DeleteConfig() did not delete file %s", filename)
		}
	})
}
