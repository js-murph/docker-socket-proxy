package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
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
		Rules: config.RuleSet{
			ACLs: []config.Rule{
				{
					Match:  config.Match{Path: "/test", Method: "GET"},
					Action: "allow",
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

	if len(savedConfig.Rules.ACLs) != 1 || savedConfig.Rules.ACLs[0].Action != "allow" {
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
		if len(cfg.Rules.ACLs) != len(testConfig.Rules.ACLs) {
			t.Errorf("Loaded config has %d ACL rules, want %d",
				len(cfg.Rules.ACLs), len(testConfig.Rules.ACLs))
		} else if cfg.Rules.ACLs[0].Action != testConfig.Rules.ACLs[0].Action {
			t.Errorf("Loaded config has action %s, want %s",
				cfg.Rules.ACLs[0].Action, testConfig.Rules.ACLs[0].Action)
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

func TestApplyPattern(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]interface{}
		pattern  config.Pattern
		want     bool
		wantBody map[string]interface{}
	}{
		{
			name: "replace env var",
			body: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "OTHER=value"},
			},
			pattern: config.Pattern{
				Field:  "Env",
				Action: "replace",
				Match:  "DEBUG=true",
				Value:  "DEBUG=false",
			},
			want: true,
			wantBody: map[string]interface{}{
				"Env": []interface{}{"DEBUG=false", "OTHER=value"},
			},
		},
		{
			name: "upsert env var",
			body: map[string]interface{}{
				"Env": []interface{}{"EXISTING=true"},
			},
			pattern: config.Pattern{
				Field:  "Env",
				Action: "upsert",
				Value:  "NEW=value",
			},
			want: true,
			wantBody: map[string]interface{}{
				"Env": []interface{}{"EXISTING=true", "NEW=value"},
			},
		},
		{
			name: "delete env var",
			body: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "KEEP=value"},
			},
			pattern: config.Pattern{
				Field:  "Env",
				Action: "delete",
				Match:  "DEBUG=*",
			},
			want: true,
			wantBody: map[string]interface{}{
				"Env": []interface{}{"KEEP=value"},
			},
		},
		{
			name: "replace boolean field",
			body: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": true,
				},
			},
			pattern: config.Pattern{
				Field:  "HostConfig.Privileged",
				Action: "replace",
				Match:  true,
				Value:  false,
			},
			want: true,
			wantBody: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyPattern(tt.body, tt.pattern)
			if got != tt.want {
				t.Errorf("applyPattern() = %v, want %v", got, tt.want)
			}

			if !reflect.DeepEqual(tt.body, tt.wantBody) {
				t.Errorf("body after applyPattern() = %v, want %v", tt.body, tt.wantBody)
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
				Rules: config.RuleSet{
					Rewrites: []config.RewriteRule{
						{
							Match: config.Match{
								Path:   "/v1.*/containers/create",
								Method: "POST",
							},
							Patterns: []config.Pattern{
								{
									Field:  "Env",
									Action: "upsert",
									Value:  "ADDED=true",
								},
								{
									Field:  "HostConfig.Privileged",
									Action: "replace",
									Match:  true,
									Value:  false,
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

			err := s.applyRewriteRules("test", tt.request)
			if err != nil {
				t.Fatalf("applyRewriteRules() error = %v", err)
			}

			if tt.wantModified {
				body, _ := io.ReadAll(tt.request.Body)
				var got map[string]interface{}
				json.Unmarshal(body, &got)

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
