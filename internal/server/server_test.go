package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"
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
	}

	srv := New(paths)

	// Create context with cancel for clean shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server with error channel
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is running
	if _, err := os.Stat(paths.Management); err != nil {
		t.Fatalf("management socket not created: %v", err)
	}

	// Clean shutdown
	cancel()
	srv.Stop()

	// Check for server errors
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server.Start() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for server to stop")
	}
}

func TestManagementHandler(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{})

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
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status OK, got %v", w.Code)
		}

		socketPath := w.Body.String()
		if socketPath == "" {
			t.Error("expected socket path in response")
		}
	})

	t.Run("delete socket", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/delete-socket", nil)
		req.Header.Set("Socket-Path", "/tmp/test.sock")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status OK, got %v", w.Code)
		}
	})
}

func TestProxyHandler(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	handler := NewProxyHandler("/tmp/docker.sock", configs, &sync.RWMutex{})

	t.Run("proxy request with ACL", func(t *testing.T) {
		configs["/tmp/test.sock"] = &config.SocketConfig{
			Rules: config.RuleSet{
				ACLs: []config.Rule{
					{
						Match:  config.Match{Path: "/v1.42/containers/json", Method: "GET"},
						Action: "allow",
					},
				},
			},
		}

		req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req, "/tmp/test.sock")

		if w.Code == http.StatusForbidden {
			t.Error("expected request to be allowed")
		}
	})

	t.Run("proxy request denied by ACL", func(t *testing.T) {
		configs["/tmp/test.sock"] = &config.SocketConfig{
			Rules: config.RuleSet{
				ACLs: []config.Rule{
					{
						Match:  config.Match{Path: "/v1.42/containers/json", Method: "GET"},
						Action: "deny",
						Reason: "not allowed",
					},
				},
			},
		}

		req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req, "/tmp/test.sock")

		if w.Code != http.StatusForbidden {
			t.Error("expected request to be denied")
		}
	})
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
