package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("/tmp", "docker-proxy-test")
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
		req, err := http.NewRequest("POST", ts.URL+"/socket/create", bytes.NewBuffer(configJSON))
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
			Status   string `json:"status"`
			Response struct {
				Socket string `json:"socket"`
			} `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Check if the socket was created
		if _, err := os.Stat(response.Response.Socket); os.IsNotExist(err) {
			t.Errorf("Socket was not created: %v", err)
		}

		// Check if the config file was created
		_, err = store.LoadConfig(response.Response.Socket)
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

		// Marshal the config to JSON
		configJSON, err := json.Marshal(minimalConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config: %v", err)
		}

		// Create a request to create a socket
		req, err := http.NewRequest("POST", ts.URL+"/socket/create", bytes.NewBuffer(configJSON))
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
			Status   string `json:"status"`
			Response struct {
				Socket string `json:"socket"`
			} `json:"response"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Check if the socket was created
		if _, err := os.Stat(response.Response.Socket); os.IsNotExist(err) {
			t.Errorf("Socket was not created: %v", err)
		}

		// Check if the config file was created
		_, err = store.LoadConfig(response.Response.Socket)
		if err != nil {
			t.Errorf("Socket config file was not created: %v", err)
		}
	})
}

func TestManagementHandler_DeleteSocket(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
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

			req := httptest.NewRequest("DELETE", "/socket/delete?socket="+tt.socketName, nil)

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
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
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
		req := httptest.NewRequest("GET", "/socket/list", nil)
		ctx := context.WithValue(req.Context(), serverContextKey, srv)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("ServeHTTP() status = %v, want %v", w.Code, http.StatusOK)
		}

		var response struct {
			Status   string `json:"status"`
			Response struct {
				Sockets []string `json:"sockets"`
			} `json:"response"`
		}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Errorf("Failed to decode response: %v", err)
		}

		// Check the response
		if response.Status != "success" {
			t.Errorf("Expected status success, got %s", response.Status)
		}
		if len(response.Response.Sockets) != 2 {
			t.Errorf("Expected 2 sockets, got %d", len(response.Response.Sockets))
		}

		// Check if the sockets are in the response
		found := make(map[string]bool)
		for _, socket := range response.Response.Sockets {
			found[socket] = true
		}

		expectedSockets := []string{"test1.sock", "test2.sock"}
		for _, expected := range expectedSockets {
			if !found[expected] {
				t.Errorf("Expected socket %s not found in response", expected)
			}
		}
	})

	// Test without server context
	t.Run("without server context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/socket/list", nil)
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
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test socket path
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a test config
	testConfig := &config.SocketConfig{
		Config: config.ConfigSet{
			PropagateSocket: "",
		},
		Rules: []config.Rule{
			{
				Match: config.Match{
					Path:   "/test",
					Method: "GET",
				},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
		},
	}

	// Create socket configs map
	socketConfigs := map[string]*config.SocketConfig{
		socketPath: testConfig,
	}

	// Create a mutex
	configMu := sync.RWMutex{}

	// Create a file store
	store := storage.NewFileStore(tmpDir)

	// Create a server instance for the context
	srv := &Server{
		socketDir:     tmpDir,
		store:         store,
		socketConfigs: socketConfigs,
		proxyServers:  make(map[string]*http.Server),
	}

	// Create a management handler using the constructor
	handler := NewManagementHandler("/tmp/docker.sock", socketConfigs, &configMu, store)

	// Test cases
	tests := []struct {
		name        string
		socketPath  string
		wantStatus  int
		wantContent string
	}{
		{
			name:       "missing socket name",
			socketPath: "",
			wantStatus: http.StatusBadRequest,
			wantContent: `{
				"status": "error",
				"response": {
					"error": "socket parameter is required"
				}
			}`,
		},
		{
			name:       "socket not found",
			socketPath: "nonexistent.sock",
			wantStatus: http.StatusNotFound,
			wantContent: `{
				"status": "error",
				"response": {
					"error": "socket not found"
				}
			}`,
		},
		{
			name:       "valid socket",
			socketPath: "test.sock",
			wantStatus: http.StatusOK,
			wantContent: `{
				"status": "success",
				"response": {
					"config": {
						"config": {
							"propagate_socket": ""
						},
						"rules": [
							{
								"match": {
									"path": "/test",
									"method": "GET"
								},
								"actions": [
									{
										"action": "allow"
									}
								]
							}
						]
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the appropriate socket path
			var req *http.Request
			if tt.socketPath == "" {
				req, _ = http.NewRequest("GET", "/socket/describe", nil)
			} else {
				req, _ = http.NewRequest("GET", "/socket/describe?socket="+url.QueryEscape(tt.socketPath), nil)
			}

			// Add server to context
			ctx := context.WithValue(req.Context(), serverContextKey, srv)
			req = req.WithContext(ctx)

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Serve the request
			handler.ServeHTTP(rr, req)

			// Check the status code
			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v, body: %v", status, tt.wantStatus, rr.Body.String())
			}

			// Check the content type
			if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Compare the JSON content
			var expected, actual interface{}
			if err := json.Unmarshal([]byte(tt.wantContent), &expected); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &actual); err != nil {
				t.Fatalf("Failed to parse actual JSON: %v", err)
			}

			if !reflect.DeepEqual(expected, actual) {
				t.Errorf("Response doesn't match expected JSON content:\nExpected: %s\nGot: %s", tt.wantContent, rr.Body.String())
			}
		})
	}
}

func TestManagementHandler_ResolveSocketPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
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
			}

			got := handler.resolveSocketPath(req, tt.socketName)
			if got != tt.want {
				t.Errorf("resolveSocketPath() = %v, want %v", got, tt.want)
			}
		})
	}
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
			body:    `{"rules":[{"match":{"path":"/test","method":"GET"},"actions":[{"action":"allow"}]}]}`,
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
			req := httptest.NewRequest("POST", "/socket/create", strings.NewReader(tt.body))
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
			}
		})
	}
}

func TestManagementHandler(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
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
			path:       "/socket/create",
			body:       strings.NewReader(`{"rules":[{"match":{"path":"/test","method":"GET"},"actions":[{"action":"allow"}]}]}`),
			headers:    map[string]string{"Content-Type": "application/json"},
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "list sockets",
			method:     "GET",
			path:       "/socket/list",
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete socket",
			method:     "DELETE",
			path:       "/socket/delete",
			headers:    map[string]string{"Socket-Path": filepath.Join(tmpDir, "test.sock")},
			withServer: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "describe socket",
			method:     "GET",
			path:       "/socket/describe",
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
			PropagateSocket: "",
		},
		Rules: []config.Rule{
			{
				Match: config.Match{
					Path:   "/test",
					Method: "GET",
				},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
		},
	}
}
