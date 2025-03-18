package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"
)

func TestManagementHandler_CreateSocket(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configs := make(map[string]*config.SocketConfig)
	store := storage.NewFileStore(tmpDir)

	// Create a server instance for the context
	srv := &Server{
		socketDir:     tmpDir,
		store:         store,
		socketConfigs: configs,
		proxyServers:  make(map[string]*http.Server),
	}

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)
	defer handler.Cleanup()

	t.Run("valid config", func(t *testing.T) {
		config := &config.SocketConfig{
			Rules: config.RuleSet{
				ACLs: []config.Rule{
					{
						Match:  config.Match{Path: "/v1.42/containers/json", Method: "GET"},
						Action: "allow",
					},
				},
			},
		}

		body, _ := json.Marshal(config)
		req := httptest.NewRequest("POST", "/create-socket", bytes.NewBuffer(body))
		ctx := context.WithValue(req.Context(), serverContextKey, srv)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status OK, got %v", w.Code)
			return
		}

		// Check if response is JSON
		contentType := w.Header().Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Logf("Response body: %s", w.Body.String())
			t.Logf("Content-Type: %s", contentType)

			// Try to extract socket path from plain text response
			socketPath := strings.TrimSpace(w.Body.String())
			if socketPath == "" {
				t.Error("Empty socket path in response")
				return
			}

			// Verify the config was stored
			storedConfig, err := store.LoadConfig(socketPath)
			if err != nil {
				t.Errorf("failed to load config: %v", err)
				return
			}

			// Compare specific fields
			if len(storedConfig.Rules.ACLs) != len(config.Rules.ACLs) {
				t.Errorf("stored config has %d ACL rules, want %d",
					len(storedConfig.Rules.ACLs), len(config.Rules.ACLs))
				return
			}

			return
		}

		var resp struct {
			SocketPath string `json:"socket_path"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Errorf("failed to unmarshal response: %v", err)
			return
		}

		// Verify the config was stored
		storedConfig, err := store.LoadConfig(resp.SocketPath)
		if err != nil {
			t.Errorf("failed to load config: %v", err)
			return
		}

		// Compare specific fields
		if len(storedConfig.Rules.ACLs) != len(config.Rules.ACLs) {
			t.Errorf("stored config has %d ACL rules, want %d",
				len(storedConfig.Rules.ACLs), len(config.Rules.ACLs))
			return
		}
	})

	tests := []struct {
		name       string
		config     *config.SocketConfig
		wantStatus int
	}{
		{
			name:       "empty config",
			config:     nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.config != nil {
				body, _ = json.Marshal(tt.config)
			}

			req := httptest.NewRequest("POST", "/create-socket", bytes.NewBuffer(body))
			// Add server to context
			ctx := context.WithValue(req.Context(), serverContextKey, srv)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				socketPath := strings.TrimSpace(w.Body.String())
				if socketPath == "" {
					t.Error("expected socket path in response")
				}

				// Verify config was persisted
				cfg, err := store.LoadConfig(socketPath)
				if err != nil {
					t.Errorf("failed to load config: %v", err)
				}
				if !reflect.DeepEqual(cfg, tt.config) {
					t.Error("stored config doesn't match original")
				}

				// Verify socket exists
				if _, err := os.Stat(socketPath); err != nil {
					t.Errorf("socket file not created: %v", err)
				}
			}
		})
	}
}

func TestManagementHandler_DeleteSocket(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	store := storage.NewFileStore("/tmp/mgmt.sock")
	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	tests := []struct {
		name       string
		socketPath string
		wantStatus int
	}{
		{
			name:       "missing socket path",
			socketPath: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid delete request",
			socketPath: "/tmp/test.sock",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/delete-socket", nil)
			if tt.socketPath != "" {
				req.Header.Set("Socket-Path", tt.socketPath)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestManagementHandler_ListSockets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configs := make(map[string]*config.SocketConfig)
	store := storage.NewFileStore(tmpDir)

	// Create a server instance for the context
	srv := &Server{
		socketDir:     tmpDir,
		store:         store,
		socketConfigs: configs,
		proxyServers:  make(map[string]*http.Server),
	}

	// Add some test configs
	socketPath1 := filepath.Join(tmpDir, "test1.sock")
	socketPath2 := filepath.Join(tmpDir, "test2.sock")

	configs[socketPath1] = &config.SocketConfig{}
	configs[socketPath2] = &config.SocketConfig{}

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	// Test with server context
	t.Run("with server context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/list-sockets", nil)
		ctx := context.WithValue(req.Context(), serverContextKey, srv)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ServeHTTP() status = %v, want %v", w.Code, http.StatusOK)
		}

		var sockets []string
		if err := json.NewDecoder(w.Body).Decode(&sockets); err != nil {
			t.Errorf("Failed to decode response: %v", err)
		}

		if len(sockets) != 2 {
			t.Errorf("Expected 2 sockets, got %d", len(sockets))
		}

		// Check that we get just the filenames
		expected := []string{"test1.sock", "test2.sock"}
		for _, s := range expected {
			found := false
			for _, actual := range sockets {
				if actual == s {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected socket %s not found in response", s)
			}
		}
	})

	// Test without server context
	t.Run("without server context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/list-sockets", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ServeHTTP() status = %v, want %v", w.Code, http.StatusOK)
		}

		var sockets []string
		if err := json.NewDecoder(w.Body).Decode(&sockets); err != nil {
			t.Errorf("Failed to decode response: %v", err)
		}

		if len(sockets) != 0 {
			t.Errorf("Expected 0 sockets, got %d", len(sockets))
		}
	})
}

func TestManagementHandler(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configs := make(map[string]*config.SocketConfig)
	store := storage.NewFileStore(tmpDir)

	srv := &Server{
		socketDir:     tmpDir,
		store:         store,
		socketConfigs: configs,
		proxyServers:  make(map[string]*http.Server),
	}

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	t.Run("create socket", func(t *testing.T) {
		config := &config.SocketConfig{
			Rules: config.RuleSet{
				ACLs: []config.Rule{
					{
						Match:  config.Match{Path: "/v1.42/containers/json", Method: "GET"},
						Action: "allow",
					},
				},
			},
		}

		body, _ := json.Marshal(config)
		req := httptest.NewRequest("POST", "/create-socket", bytes.NewBuffer(body))
		ctx := context.WithValue(req.Context(), serverContextKey, srv)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status OK, got %v", w.Code)
		}
	})

	// ... rest of the test
}
