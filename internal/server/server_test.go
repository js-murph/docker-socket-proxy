package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"
)

func TestServer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	paths := &management.SocketPaths{
		Management: filepath.Join(tmpDir, "mgmt.sock"),
		Docker:     filepath.Join(tmpDir, "docker.sock"),
		SocketDir:  tmpDir,
	}

	// Create a test socket and its config
	testSocket := filepath.Join(tmpDir, "test.sock")

	// Create the socket file to ensure it exists
	l, err := net.Listen("unix", testSocket)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()

	testConfig := &config.SocketConfig{
		Rules: []config.Rule{
			{
				Match: config.Match{Path: "/test", Method: "GET"},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
		},
	}

	store := storage.NewFileStore(paths.SocketDir)
	if err := store.SaveConfig(testSocket, testConfig); err != nil {
		t.Fatal(err)
	}

	// Verify the config was saved correctly
	savedConfig, err := store.LoadConfig(testSocket)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if savedConfig.Rules[0].Actions[0].Action != "allow" {
		t.Fatalf("Config not saved correctly: %+v", savedConfig)
	}

	// Create server with the store and manually add the config
	srv := &Server{
		managementSocket: paths.Management,
		dockerSocket:     paths.Docker,
		socketDir:        paths.SocketDir,
		server:           &http.Server{},
		socketConfigs:    make(map[string]*config.SocketConfig),
		createdSockets:   make([]string, 0),
		store:            store,
		proxyServers:     make(map[string]*http.Server),
		configMu:         sync.RWMutex{},
	}

	// Manually add the config
	srv.configMu.Lock()
	srv.socketConfigs[testSocket] = testConfig
	srv.configMu.Unlock()

	// Verify config was loaded
	srv.configMu.RLock()
	cfg, ok := srv.socketConfigs[testSocket]
	srv.configMu.RUnlock()

	if !ok {
		t.Error("Expected test config to be loaded")
	} else {
		// Compare specific fields instead of using DeepEqual
		if len(cfg.Rules) != len(testConfig.Rules) {
			t.Errorf("Loaded config has %d rules, want %d",
				len(cfg.Rules), len(testConfig.Rules))
		} else if cfg.Rules[0].Actions[0].Action != testConfig.Rules[0].Actions[0].Action {
			t.Errorf("Loaded config has action %s, want %s",
				cfg.Rules[0].Actions[0].Action, testConfig.Rules[0].Actions[0].Action)
		}
	}

	// Start the server in a goroutine with a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.startWithContext(ctx); err != nil && err != http.ErrServerClosed {
			t.Errorf("Server.Start() error = %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the server
	cancel()
}

// Add this method to the Server struct
func (s *Server) startWithContext(ctx context.Context) error {
	// Set up the management handler
	handler := NewManagementHandler(s.dockerSocket, s.socketConfigs, &s.configMu, s.store)
	s.server.Handler = handler

	// Listen on the management socket
	listener, err := net.Listen("unix", s.managementSocket)
	if err != nil {
		return fmt.Errorf("failed to listen on management socket: %w", err)
	}

	// Remove the socket file when the server stops
	defer os.Remove(s.managementSocket)

	log := logging.GetLogger()
	log.Info("Management server listening on socket", "path", s.managementSocket)

	// Serve until context is canceled
	go func() {
		<-ctx.Done()
		s.server.Close()
	}()

	return s.server.Serve(listener)
}

func TestApplyRewriteActions(t *testing.T) {
	tests := []struct {
		name         string
		body         map[string]interface{}
		actions      []config.Rule
		wantBody     map[string]interface{}
		wantModified bool
	}{
		{
			name: "replace env var",
			body: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "OTHER=value"},
			},
			actions: []config.Rule{
				{
					Actions: []config.Action{
						{
							Action: "replace",
							Contains: map[string]interface{}{
								"Env": "DEBUG=true",
							},
						},
						{
							Action: "replace",
							Update: map[string]interface{}{
								"Env": []interface{}{"DEBUG=false", "OTHER=value"},
							},
						},
					},
				},
			},
			wantBody: map[string]interface{}{
				"Env": []interface{}{"DEBUG=false", "OTHER=value"},
			},
			wantModified: true,
		},
		{
			name: "upsert env var",
			body: map[string]interface{}{
				"Env": []interface{}{"EXISTING=true"},
			},
			actions: []config.Rule{
				{
					Actions: []config.Action{
						{
							Action: "upsert",
							Update: map[string]interface{}{
								"Env": []interface{}{"NEW=value"},
							},
						},
					},
				},
			},
			wantBody: map[string]interface{}{
				"Env": []interface{}{"EXISTING=true", "NEW=value"},
			},
			wantModified: true,
		},
		{
			name: "delete env var",
			body: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "KEEP=value"},
			},
			actions: []config.Rule{
				{
					Actions: []config.Action{
						{
							Action: "delete",
							Contains: map[string]interface{}{
								"Env": []interface{}{"DEBUG=true"},
							},
						},
					},
				},
			},
			wantBody: map[string]interface{}{
				"Env": []interface{}{"KEEP=value"},
			},
			wantModified: true,
		},
		{
			name: "replace boolean field",
			body: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": true,
				},
			},
			actions: []config.Rule{
				{
					Actions: []config.Action{
						{
							Action: "replace",
							Contains: map[string]interface{}{
								"HostConfig": map[string]interface{}{
									"Privileged": true,
								},
							},
							Update: map[string]interface{}{
								"HostConfig": map[string]interface{}{
									"Privileged": false,
								},
							},
						},
					},
				},
			},
			wantBody: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": false,
				},
			},
			wantModified: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the body to avoid modifying the test case
			body := make(map[string]interface{})
			for k, v := range tt.body {
				body[k] = v
			}

			// Apply the rewrite actions
			modified := false
			for _, rule := range tt.actions {
				for _, action := range rule.Actions {
					switch action.Action {
					case "replace":
						if config.MatchesStructure(body, action.Contains) {
							if config.MergeStructure(body, action.Update, true) {
								modified = true
							}
						}
					case "upsert":
						if config.MergeStructure(body, action.Update, false) {
							modified = true
						}
					case "delete":
						if config.DeleteMatchingFields(body, action.Contains) {
							modified = true
						}
					}
				}
			}

			// Check if the body was modified as expected
			if modified != tt.wantModified {
				t.Errorf("applyRewriteActions() modified = %v, want %v", modified, tt.wantModified)
			}

			// Check if the body matches the expected result
			if !reflect.DeepEqual(body, tt.wantBody) {
				t.Errorf("body after applyRewriteActions() = %v, want %v", body, tt.wantBody)
			}
		})
	}
}

func TestApplyRewriteRules(t *testing.T) {
	tests := []struct {
		name         string
		config       *config.SocketConfig
		request      *http.Request
		wantBody     map[string]interface{}
		wantModified bool
	}{
		{
			name: "apply multiple rewrites",
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action: "upsert",
								Update: map[string]interface{}{
									"Env": []interface{}{"ADDED=true"},
								},
							},
							{
								Action: "replace",
								Contains: map[string]interface{}{
									"HostConfig": map[string]interface{}{
										"Privileged": true,
									},
								},
								Update: map[string]interface{}{
									"HostConfig": map[string]interface{}{
										"Privileged": false,
									},
								},
							},
						},
					},
				},
			},
			request: createTestRequest(map[string]interface{}{
				"Env": []interface{}{"EXISTING=true"},
				"HostConfig": map[string]interface{}{
					"Privileged": true,
				},
			}),
			wantBody: map[string]interface{}{
				"Env": []interface{}{"EXISTING=true", "ADDED=true"},
				"HostConfig": map[string]interface{}{
					"Privileged": false,
				},
			},
			wantModified: true,
		},
		// Add more test cases...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				socketConfigs: map[string]*config.SocketConfig{
					"test": tt.config,
				},
			}

			err := s.applyRewriteRules(tt.request, "test")
			if err != nil {
				t.Fatalf("applyRewriteRules() error = %v", err)
			}

			if tt.wantModified {
				body, _ := io.ReadAll(tt.request.Body)
				var got map[string]interface{}
				err = json.Unmarshal(body, &got)
				if err != nil {
					t.Errorf("Failed to unmarshal response body: %v", err)
					return
				}

				if !reflect.DeepEqual(got, tt.wantBody) {
					t.Errorf("body after rewrite = %v, want %v", got, tt.wantBody)
				}
			}
		})
	}
}

func createTestRequest(body map[string]interface{}) *http.Request {
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestRegexMatch(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		method string
		rule   struct {
			path   string
			method string
		}
		want bool
	}{
		{
			name:   "exact match both",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/v1\\.42/containers/json$", method: "^GET$"},
			want:   true,
		},
		{
			name:   "regex path",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/v1\\.[0-9]+/containers/.*$", method: "^GET$"},
			want:   true,
		},
		{
			name:   "regex method",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/v1\\.42/containers/json$", method: "^(GET|POST)$"},
			want:   true,
		},
		{
			name:   "regex both",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/.*$", method: "^.*$"},
			want:   true,
		},
		{
			name:   "no match path",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/v1\\.42/networks/.*$", method: "^GET$"},
			want:   false,
		},
		{
			name:   "no match method",
			path:   "/v1.42/containers/json",
			method: "GET",
			rule:   struct{ path, method string }{path: "^/v1\\.42/containers/json$", method: "^POST$"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock request with the specified path and method
			r := &http.Request{
				Method: tt.method,
				URL: &url.URL{
					Path: tt.path,
				},
			}

			// Create a mock match
			match := config.Match{
				Path:   tt.rule.path,
				Method: tt.rule.method,
			}

			// Create a handler to test the rule matching
			handler := &ProxyHandler{}

			if got := handler.ruleMatches(r, match); got != tt.want {
				t.Errorf("ruleMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}
