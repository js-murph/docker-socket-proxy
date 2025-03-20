package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"
)

func TestManagementHandler_CreateSocket(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "docker-proxy-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test config
	testConfig := createTestConfig()

	// Create a test server with context
	store := storage.NewFileStore(tempDir)
	configs := make(map[string]*config.SocketConfig)

	// Create a mock server
	mockServer := &Server{
		socketDir:     tempDir,
		store:         store,
		socketConfigs: configs,
		proxyServers:  make(map[string]*http.Server),
	}

	// Create the handler
	handler := NewManagementHandler(tempDir, configs, &sync.RWMutex{}, store)

	// Create a server that injects the mock server into the context
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), serverContextKey, mockServer)
		handler.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer ts.Close()

	// Test creating a socket with a valid config
	t.Run("valid_config", func(t *testing.T) {
		// Marshal the config to JSON
		configJSON, err := json.Marshal(testConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config: %v", err)
		}

		// Create a request to create a socket
		req, err := http.NewRequest("POST", ts.URL+"/socket", bytes.NewBuffer(configJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Check the response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status OK, got %v: %s", resp.Status, body)
		}

		// Parse the response
		var response struct {
			SocketPath string `json:"socket_path"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Check if the socket was created
		if _, err := os.Stat(response.SocketPath); os.IsNotExist(err) {
			t.Errorf("Socket was not created: %v", err)
		}

		// Check if the config file was created
		_, err = store.LoadConfig(response.SocketPath)
		if err != nil {
			t.Errorf("Socket config file was not created: %v", err)
		}
	})

	// Test creating a socket with an empty config
	t.Run("empty_config", func(t *testing.T) {
		// Create a minimal valid config
		minimalConfig := &config.SocketConfig{
			Config: config.ConfigSet{
				PropagateSocket: "/var/run/docker.sock",
			},
			Rules: config.RuleSet{
				ACLs: []config.Rule{
					{
						Match: config.Match{
							Path:   "/.*",
							Method: ".*",
						},
						Action: "allow",
					},
				},
			},
		}

		// Marshal the config to JSON
		configJSON, err := json.Marshal(minimalConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config: %v", err)
		}

		// Create a request to create a socket
		req, err := http.NewRequest("POST", ts.URL+"/socket", bytes.NewBuffer(configJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Check the response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status OK, got %v: %s", resp.Status, body)
		}

		// Parse the response
		var response struct {
			SocketPath string `json:"socket_path"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Check if the socket was created
		if _, err := os.Stat(response.SocketPath); os.IsNotExist(err) {
			t.Errorf("Socket was not created: %v", err)
		}

		// Check if the config file was created
		_, err = store.LoadConfig(response.SocketPath)
		if err != nil {
			t.Errorf("Socket config file was not created: %v", err)
		}
	})
}

func TestManagementHandler_DeleteSocket(t *testing.T) {
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

	// Create a test socket
	socketPath := filepath.Join(tmpDir, "test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()

	// Add the socket to the configs
	configs[socketPath] = &config.SocketConfig{}

	// Save the config
	if err := store.SaveConfig(socketPath, configs[socketPath]); err != nil {
		t.Fatal(err)
	}

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	tests := []struct {
		name       string
		socketName string
		useHeader  bool
		withServer bool
		wantStatus int
	}{
		{
			name:       "missing socket name",
			socketName: "",
			useHeader:  false,
			withServer: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid socket with query param",
			socketName: "test.sock",
			useHeader:  false,
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid socket with header",
			socketName: "test.sock",
			useHeader:  true,
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "nonexistent socket",
			socketName: "nonexistent.sock",
			useHeader:  false,
			withServer: true,
			wantStatus: http.StatusOK, // We don't return an error for nonexistent sockets
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate the socket for each test if it's the valid socket test
			if tt.socketName == "test.sock" && !strings.Contains(tt.name, "nonexistent") {
				listener, err := net.Listen("unix", socketPath)
				if err != nil {
					t.Fatal(err)
				}
				listener.Close()

				// Re-add the socket to the configs
				configs[socketPath] = &config.SocketConfig{}

				// Re-save the config
				if err := store.SaveConfig(socketPath, configs[socketPath]); err != nil {
					t.Fatal(err)
				}
			}

			req := httptest.NewRequest("DELETE", "/socket/"+tt.socketName, nil)

			// Add socket name as query param or header
			if tt.socketName != "" {
				if tt.useHeader {
					req.Header.Set("Socket-Path", tt.socketName)
				} else {
					q := req.URL.Query()
					q.Add("socket", tt.socketName)
					req.URL.RawQuery = q.Encode()
				}
			}

			// Add server to context if needed
			if tt.withServer {
				ctx := context.WithValue(req.Context(), serverContextKey, srv)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v, body: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}

			// Verify the socket was deleted if it was a valid delete request
			if tt.wantStatus == http.StatusOK && tt.socketName == "test.sock" {
				// Check if the socket file was removed
				if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
					t.Errorf("Socket file still exists after deletion")
				}

				// Check if the config was removed from the map
				if _, exists := configs[socketPath]; exists {
					t.Errorf("Socket config still exists in map after deletion")
				}

				// Check if the config file was deleted
				if _, err := store.LoadConfig(socketPath); err == nil {
					t.Errorf("Socket config file still exists after deletion")
				}
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

func TestManagementHandler_DescribeSocket(t *testing.T) {
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

	// Create a test socket config
	socketPath := filepath.Join(tmpDir, "test.sock")
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

	// Save the config
	if err := store.SaveConfig(socketPath, testConfig); err != nil {
		t.Fatal(err)
	}

	// Add the config to the map
	configs[socketPath] = testConfig

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	tests := []struct {
		name       string
		socketName string
		withServer bool
		wantStatus int
	}{
		{
			name:       "missing socket name",
			socketName: "",
			withServer: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "socket not found",
			socketName: "nonexistent.sock",
			withServer: true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "valid socket with server context",
			socketName: "test.sock",
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid socket without server context",
			socketName: filepath.Base(socketPath), // Use just the filename
			withServer: false,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/describe-socket", nil)

			// Add query parameter
			if tt.socketName != "" {
				q := req.URL.Query()
				q.Add("socket", tt.socketName)
				req.URL.RawQuery = q.Encode()
			}

			// Add server to context if needed
			if tt.withServer {
				ctx := context.WithValue(req.Context(), serverContextKey, srv)
				req = req.WithContext(ctx)
			} else {
				// For tests without server context, we need to set the socket directory
				// in the environment so the handler can find the socket
				oldEnv := os.Getenv("DOCKER_PROXY_SOCKET_DIR")
				os.Setenv("DOCKER_PROXY_SOCKET_DIR", tmpDir)
				defer os.Setenv("DOCKER_PROXY_SOCKET_DIR", oldEnv)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v, body: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantStatus == http.StatusOK {
				// Check that the response is YAML
				contentType := w.Header().Get("Content-Type")
				if contentType != "application/yaml" {
					t.Errorf("Expected Content-Type application/yaml, got %s", contentType)
				}

				// Verify the YAML contains expected content
				responseBody := w.Body.String()
				if !strings.Contains(responseBody, "rules:") ||
					!strings.Contains(responseBody, "acls:") ||
					!strings.Contains(responseBody, "action: allow") {
					t.Errorf("Response doesn't contain expected YAML content: %s", responseBody)
				}
			}
		})
	}
}

func TestManagementHandler_ResolveSocketPath(t *testing.T) {
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

	tests := []struct {
		name       string
		socketName string
		withServer bool
		want       string
	}{
		{
			name:       "relative path with server context",
			socketName: "test.sock",
			withServer: true,
			want:       filepath.Join(tmpDir, "test.sock"),
		},
		{
			name:       "absolute path with server context",
			socketName: "/var/run/test.sock",
			withServer: true,
			want:       "/var/run/test.sock",
		},
		{
			name:       "relative path without server context",
			socketName: "test.sock",
			withServer: false,
			want:       "/var/run/docker-proxy/test.sock", // Default directory
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)

			if tt.withServer {
				ctx := context.WithValue(req.Context(), serverContextKey, srv)
				req = req.WithContext(ctx)
			} else {
				// Reset environment variable for test
				oldEnv := os.Getenv("DOCKER_PROXY_SOCKET_DIR")
				os.Setenv("DOCKER_PROXY_SOCKET_DIR", "")
				defer os.Setenv("DOCKER_PROXY_SOCKET_DIR", oldEnv)
			}

			got := handler.resolveSocketPath(req, tt.socketName)
			if got != tt.want {
				t.Errorf("resolveSocketPath() = %v, want %v", got, tt.want)
			}
		})
	}

	// Test with custom environment variable
	t.Run("with custom environment variable", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		// Set custom environment variable
		oldEnv := os.Getenv("DOCKER_PROXY_SOCKET_DIR")
		os.Setenv("DOCKER_PROXY_SOCKET_DIR", "/custom/path")
		defer os.Setenv("DOCKER_PROXY_SOCKET_DIR", oldEnv)

		got := handler.resolveSocketPath(req, "test.sock")
		want := "/custom/path/test.sock"
		if got != want {
			t.Errorf("resolveSocketPath() = %v, want %v", got, want)
		}
	})
}

func TestManagementHandler_ValidateAndDecodeConfig(t *testing.T) {
	handler := NewManagementHandler("/tmp/docker.sock", make(map[string]*config.SocketConfig), &sync.RWMutex{}, nil)

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "empty body",
			body:    "",
			wantErr: false, // Empty body is allowed, will use default config
		},
		{
			name:    "valid config",
			body:    `{"rules":{"acls":[{"match":{"path":"/test","method":"GET"},"action":"allow"}]}}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `{"rules":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/create-socket", strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			config, err := handler.validateAndDecodeConfig(req)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateAndDecodeConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if config == nil {
					t.Errorf("validateAndDecodeConfig() returned nil config for valid input")
				}

				if tt.body == "" {
					// For empty body, we should get a default config
					if config.Rules.ACLs == nil {
						t.Errorf("validateAndDecodeConfig() did not create default config for empty body")
					}
				} else {
					// For valid JSON, we should get the config we provided
					if len(config.Rules.ACLs) != 1 || config.Rules.ACLs[0].Action != "allow" {
						t.Errorf("validateAndDecodeConfig() did not parse config correctly: %+v", config)
					}
				}
			}
		})
	}
}

func TestManagementHandler(t *testing.T) {
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
		configMu:      sync.RWMutex{},
	}

	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{}, store)

	tests := []struct {
		name       string
		method     string
		path       string
		body       io.Reader
		headers    map[string]string
		withServer bool
		wantStatus int
	}{
		{
			name:       "create socket",
			method:     "POST",
			path:       "/create-socket",
			body:       strings.NewReader(`{"rules":{"acls":[{"match":{"path":"/test","method":"GET"},"action":"allow"}]}}`),
			headers:    map[string]string{"Content-Type": "application/json"},
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "list sockets",
			method:     "GET",
			path:       "/list-sockets",
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete socket",
			method:     "DELETE",
			path:       "/delete-socket",
			headers:    map[string]string{"Socket-Path": filepath.Join(tmpDir, "test.sock")},
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "describe socket",
			method:     "GET",
			path:       "/describe-socket",
			headers:    map[string]string{},
			withServer: true,
			wantStatus: http.StatusBadRequest, // Missing socket name
		},
		{
			name:       "unknown request",
			method:     "GET",
			path:       "/unknown",
			withServer: true,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, tt.body)

			// Add headers
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// Add server to context if needed
			if tt.withServer {
				ctx := context.WithValue(req.Context(), serverContextKey, srv)
				req = req.WithContext(ctx)
			}

			// For delete socket test, create a test socket first
			if tt.name == "delete socket" {
				socketPath := filepath.Join(tmpDir, "test.sock")
				listener, err := net.Listen("unix", socketPath)
				if err != nil {
					t.Fatal(err)
				}
				listener.Close()

				// Add the socket to the configs
				configs[socketPath] = &config.SocketConfig{}

				// Save the config
				if err := store.SaveConfig(socketPath, configs[socketPath]); err != nil {
					t.Fatal(err)
				}
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v, body: %s",
					w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// createTestConfig creates a test config with valid rules
func createTestConfig() *config.SocketConfig {
	return &config.SocketConfig{
		Config: config.ConfigSet{
			PropagateSocket: "/var/run/docker.sock",
		},
		Rules: config.RuleSet{
			ACLs: []config.Rule{
				{
					Match: config.Match{
						Path:   "/v1.*/containers/json",
						Method: "GET",
					},
					Action: "allow",
				},
				{
					Match: config.Match{
						Path:   "/v1.*/containers/create",
						Method: "POST",
					},
					Action: "deny",
					Reason: "Test deny rule", // Add reason for deny rule
				},
				{
					Match: config.Match{
						Path:   "/.*",
						Method: ".*",
					},
					Action: "allow",
				},
			},
		},
	}
}
