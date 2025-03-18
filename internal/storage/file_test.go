package storage

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
)

func TestFileStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgmtSocket := filepath.Join(tmpDir, "mgmt.sock")
	store := NewFileStore(mgmtSocket)

	// Create test socket and config
	testSocket := filepath.Join(tmpDir, "test.sock")
	l, err := net.Listen("unix", testSocket)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()

	testConfig := &config.SocketConfig{
		Rules: config.RuleSet{
			ACLs: []config.Rule{
				{
					Match:  config.Match{Path: "/test", Method: "GET"},
					Action: "allow",
				},
			},
		},
	}

	// Test SaveConfig and LoadConfig
	t.Run("save and load config", func(t *testing.T) {
		if err := store.SaveConfig(testSocket, testConfig); err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}

		loaded, err := store.LoadConfig(testSocket)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		if !reflect.DeepEqual(loaded, testConfig) {
			t.Errorf("LoadConfig() got = %+v, want %+v", loaded, testConfig)
		}
	})

	// Test LoadExistingConfigs
	t.Run("load existing configs", func(t *testing.T) {
		configs, err := store.LoadExistingConfigs()
		if err != nil {
			t.Fatalf("LoadExistingConfigs() error = %v", err)
		}

		if len(configs) != 1 {
			t.Errorf("LoadExistingConfigs() got %d configs, want 1", len(configs))
		}

		if cfg, ok := configs[testSocket]; !ok {
			t.Error("LoadExistingConfigs() missing test socket config")
		} else if !reflect.DeepEqual(cfg, testConfig) {
			t.Errorf("LoadExistingConfigs() got = %+v, want %+v", cfg, testConfig)
		}
	})

	// Test DeleteConfig
	t.Run("delete config", func(t *testing.T) {
		if err := store.DeleteConfig(testSocket); err != nil {
			t.Fatalf("DeleteConfig() error = %v", err)
		}

		if _, err := os.Stat(store.configPath(testSocket)); !os.IsNotExist(err) {
			t.Error("DeleteConfig() config file still exists")
		}
	})
}
