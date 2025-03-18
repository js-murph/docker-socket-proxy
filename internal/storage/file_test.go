package storage

import (
	"net"
	"os"
	"path/filepath"
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

		loadedConfig, err := store.LoadConfig(testSocket)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		// Compare specific fields instead of using DeepEqual
		if len(loadedConfig.Rules.ACLs) != len(testConfig.Rules.ACLs) {
			t.Errorf("LoadConfig() got %d ACL rules, want %d",
				len(loadedConfig.Rules.ACLs), len(testConfig.Rules.ACLs))
			return
		}

		if loadedConfig.Rules.ACLs[0].Action != testConfig.Rules.ACLs[0].Action {
			t.Errorf("LoadConfig() got action %s, want %s",
				loadedConfig.Rules.ACLs[0].Action, testConfig.Rules.ACLs[0].Action)
		}

		if loadedConfig.Rules.ACLs[0].Match.Path != testConfig.Rules.ACLs[0].Match.Path {
			t.Errorf("LoadConfig() got path %s, want %s",
				loadedConfig.Rules.ACLs[0].Match.Path, testConfig.Rules.ACLs[0].Match.Path)
		}

		if loadedConfig.Rules.ACLs[0].Match.Method != testConfig.Rules.ACLs[0].Match.Method {
			t.Errorf("LoadConfig() got method %s, want %s",
				loadedConfig.Rules.ACLs[0].Match.Method, testConfig.Rules.ACLs[0].Match.Method)
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
			return
		}

		cfg, ok := configs[testSocket]
		if !ok {
			t.Error("LoadExistingConfigs() missing test socket config")
			return
		}

		// Compare specific fields
		if len(cfg.Rules.ACLs) != len(testConfig.Rules.ACLs) {
			t.Errorf("LoadExistingConfigs() got %d ACL rules, want %d",
				len(cfg.Rules.ACLs), len(testConfig.Rules.ACLs))
			return
		}

		if cfg.Rules.ACLs[0].Action != testConfig.Rules.ACLs[0].Action {
			t.Errorf("LoadExistingConfigs() got action %s, want %s",
				cfg.Rules.ACLs[0].Action, testConfig.Rules.ACLs[0].Action)
		}
	})

	// Test DeleteConfig
	t.Run("delete config", func(t *testing.T) {
		if err := store.DeleteConfig(testSocket); err != nil {
			t.Fatalf("DeleteConfig() error = %v", err)
		}

		_, err := store.LoadConfig(testSocket)
		if err == nil {
			t.Error("LoadConfig() expected error after deletion")
		}
	})
}
