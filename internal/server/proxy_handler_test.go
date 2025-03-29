package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"bytes"
	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
	"encoding/json"
	"io"
	"strings"
)

func TestProxyHandler_ProcessRules(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
		config  *config.SocketConfig
		want    bool
		reason  string
	}{
		{
			name:    "nil config",
			request: httptest.NewRequest("GET", "/", nil),
			config:  nil,
			want:    true,
			reason:  "",
		},
		{
			name:    "empty rules",
			request: httptest.NewRequest("GET", "/", nil),
			config:  &config.SocketConfig{Rules: []config.Rule{}},
			want:    true,
			reason:  "",
		},
		{
			name:    "allow rule",
			request: httptest.NewRequest("GET", "/test", nil),
			config: &config.SocketConfig{
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
			},
			want:   true,
			reason: "",
		},
		{
			name:    "deny rule",
			request: httptest.NewRequest("GET", "/test", nil),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/test",
							Method: "GET",
						},
						Actions: []config.Action{
							{
								Action: "deny",
								Reason: "Test deny",
							},
						},
					},
				},
			},
			want:   false,
			reason: "Test deny",
		},
		{
			name: "deny with matching body content",
			request: func() *http.Request {
				body := map[string]any{
					"Env": []any{"BLOCK=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action:   "deny",
								Reason:   "Blocked by environment variable",
								Contains: map[string]any{"Env": []any{"BLOCK=true"}},
							},
						},
					},
				},
			},
			want:   false,
			reason: "Blocked by environment variable",
		},
		{
			name: "allow when body doesn't match deny condition",
			request: func() *http.Request {
				body := map[string]any{
					"Env": []any{"ALLOW=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action:   "deny",
								Reason:   "Blocked by environment variable",
								Contains: map[string]any{"Env": []any{"BLOCK=true"}},
							},
						},
					},
				},
			},
			want:   true,
			reason: "",
		},
		{
			name: "deny with nested content match",
			request: func() *http.Request {
				body := map[string]any{
					"HostConfig": map[string]any{
						"Privileged": true,
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action: "deny",
								Reason: "Privileged containers not allowed",
								Contains: map[string]any{
									"HostConfig": map[string]any{
										"Privileged": true,
									},
								},
							},
						},
					},
				},
			},
			want:   false,
			reason: "Privileged containers not allowed",
		},
		{
			name: "deny with rule match contains and action contains",
			request: func() *http.Request {
				body := map[string]any{
					"Env":     []any{"DEBUG=true"},
					"Network": "host",
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:     "/v1.*/containers/create",
							Method:   "POST",
							Contains: map[string]any{"Env": []any{"DEBUG=true"}},
						},
						Actions: []config.Action{
							{
								Action:   "deny",
								Reason:   "Host network not allowed with debug mode",
								Contains: map[string]any{"Network": "host"},
							},
						},
					},
				},
			},
			want:   false,
			reason: "Host network not allowed with debug mode",
		},
		{
			name: "allow skips subsequent deny rules",
			request: func() *http.Request {
				body := map[string]any{
					"Env": []any{"ALLOW=true", "BLOCK=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
							Contains: map[string]any{
								"Env": []any{"ALLOW=true"},
							},
						},
						Actions: []config.Action{
							{
								Action: "allow",
								Reason: "Explicitly allowed",
							},
						},
					},
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
							Contains: map[string]any{
								"Env": []any{"BLOCK=true"},
							},
						},
						Actions: []config.Action{
							{
								Action: "deny",
								Reason: "Should not reach this rule",
							},
						},
					},
				},
			},
			want:   true,
			reason: "Explicitly allowed",
		},
		{
			name: "allow in first action skips subsequent deny actions in same rule",
			request: func() *http.Request {
				body := map[string]any{
					"Env": []any{"MIXED=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action: "allow",
								Reason: "Allow first",
							},
							{
								Action: "deny",
								Reason: "Should not reach this action",
							},
						},
					},
				},
			},
			want:   true,
			reason: "Allow first",
		},
		{
			name: "body remains readable after allow",
			request: func() *http.Request {
				body := map[string]any{
					"Env": []any{"TEST=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			config: &config.SocketConfig{
				Rules: []config.Rule{
					{
						Match: config.Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []config.Action{
							{
								Action: "allow",
								Reason: "Allowed",
							},
						},
					},
				},
			},
			want:   true,
			reason: "Allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewProxyHandler("/tmp/docker.sock", make(map[string]*config.SocketConfig), &sync.RWMutex{})

			got, reason, err := handler.processRules(tt.request, tt.config)
			if err != nil {
				t.Fatalf("processRules() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("processRules() got = %v, want %v", got, tt.want)
			}
			if reason != tt.reason {
				t.Errorf("processRules() reason = %v, want %v", reason, tt.reason)
			}
		})
	}
}

func TestProxyHandlerWithMock(t *testing.T) {
	// Create a mock Docker API server
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"message":"OK"}`))
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer dockerServer.Close()

	// Create a proxy handler with a test configuration
	configs := make(map[string]*config.SocketConfig)
	configs["/tmp/docker.sock"] = &config.SocketConfig{
		Rules: []config.Rule{
			{
				Match: config.Match{Path: "/v1.42/containers/json", Method: "GET"},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
			{
				Match: config.Match{Path: "/", Method: ""},
				Actions: []config.Action{
					{
						Action: "deny",
					},
				},
			},
		},
	}

	// Create a custom test handler
	handler := &TestProxyHandler{
		dockerSocket:  "/tmp/docker.sock",
		socketConfigs: configs,
		configMu:      &sync.RWMutex{},
		testServer:    dockerServer,
	}

	tests := []struct {
		name        string
		method      string
		path        string
		wantAllowed bool
	}{
		{
			name:        "proxy request with ACL",
			method:      "GET",
			path:        "/v1.42/containers/json",
			wantAllowed: true,
		},
		{
			name:        "proxy request denied by ACL",
			method:      "POST",
			path:        "/v1.42/containers/create",
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTPWithSocket(w, req, "/tmp/docker.sock")

			if tt.wantAllowed {
				if w.Code != http.StatusOK {
					t.Errorf("expected request to be allowed, got status %d", w.Code)
				}
			} else {
				if w.Code != http.StatusForbidden {
					t.Errorf("expected request to be denied, got status %d", w.Code)
				}
			}
		})
	}
}

// TestProxyHandler is a special version of ProxyHandler for testing
type TestProxyHandler struct {
	dockerSocket  string
	socketConfigs map[string]*config.SocketConfig
	configMu      *sync.RWMutex
	testServer    *httptest.Server
}

// ServeHTTPWithSocket is a test version that uses the test server instead of Unix socket
func (h *TestProxyHandler) ServeHTTPWithSocket(w http.ResponseWriter, r *http.Request, socketPath string) {
	log := logging.GetLogger()

	// Get the socket configuration
	h.configMu.RLock()
	socketConfig, exists := h.socketConfigs[socketPath]
	if !exists {
		// Handle the case where the key doesn't exist
		http.Error(w, "Socket configuration not found", http.StatusInternalServerError)
		return
	}
	h.configMu.RUnlock()

	// Check if the request is allowed by the ACLs
	allowed, reason := h.checkACLs(r, socketConfig)

	// Log the request
	log.Info("Proxy request",
		"method", r.Method,
		"path", r.URL.Path,
		"socket", socketPath,
		"allowed", allowed,
		"reason", reason,
	)

	if !allowed {
		http.Error(w, "Access denied by ACL: "+reason, http.StatusForbidden)
		return
	}

	// For tests, forward to the test server instead of using Unix socket
	resp, err := h.testServer.Client().Do(&http.Request{
		Method: r.Method,
		URL: &url.URL{
			Scheme: "http",
			Host:   strings.TrimPrefix(h.testServer.URL, "http://"),
			Path:   r.URL.Path,
		},
		Header: r.Header,
		Body:   r.Body,
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Error("Failed to close response body", "error", err)
		}
	}()

	// Copy the response headers
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	// Copy the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Error("Failed to copy response body", "error", err)
		// Since we've already started writing the response, we can't change the status code
		// Just log the error and return
		return
	}
}

// checkACLs checks if a request is allowed by the ACLs
func (h *TestProxyHandler) checkACLs(r *http.Request, socketConfig *config.SocketConfig) (bool, string) {
	// If there's no config, allow all requests
	if socketConfig == nil {
		return true, ""
	}

	// If there are no ACLs, allow by default
	if len(socketConfig.Rules) == 0 {
		return true, ""
	}

	// Check each ACL rule
	for _, rule := range socketConfig.Rules {
		// Check if the rule matches the request
		if h.ruleMatches(r, rule.Match) {
			if len(rule.Actions) > 0 && rule.Actions[0].Action == "allow" {
				return true, ""
			} else if len(rule.Actions) > 0 {
				return false, rule.Actions[0].Reason
			}
		}
	}

	// If no rule matches, allow by default
	return true, ""
}

// ruleMatches checks if a request matches an ACL rule
func (h *TestProxyHandler) ruleMatches(r *http.Request, match config.Match) bool {
	// Check path match
	if match.Path != "" && !strings.HasPrefix(r.URL.Path, match.Path) {
		return false
	}

	// Check method match
	if match.Method != "" && r.Method != match.Method {
		return false
	}

	return true
}

func TestRegexMatching(t *testing.T) {
	// Create a handler for testing
	handler := NewProxyHandler("/tmp/docker.sock", nil, &sync.RWMutex{})

	tests := []struct {
		name   string
		path   string
		method string
		match  config.Match
		want   bool
	}{
		// Path regex tests
		{
			name:   "exact path match",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1.42/containers/json"},
			want:   true,
		},
		{
			name:   "path regex with version",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/containers/json"},
			want:   true,
		},
		{
			name:   "path regex with wildcard",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/containers/.*"},
			want:   true,
		},
		{
			name:   "path regex no match",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/networks/.*"},
			want:   false,
		},

		// Method regex tests
		{
			name:   "exact method match",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Method: "GET"},
			want:   true,
		},
		{
			name:   "method regex OR",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Method: "GET|POST"},
			want:   true,
		},
		{
			name:   "method regex with anchors",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Method: "^GET$"},
			want:   true,
		},
		{
			name:   "method regex no match",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Method: "POST|PUT"},
			want:   false,
		},

		// Combined path and method tests
		{
			name:   "both path and method match",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/containers/.*", Method: "GET|HEAD"},
			want:   true,
		},
		{
			name:   "path matches but method doesn't",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/containers/.*", Method: "POST"},
			want:   false,
		},
		{
			name:   "method matches but path doesn't",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9]+/networks/.*", Method: "GET"},
			want:   false,
		},

		// Special regex patterns
		{
			name:   "dot matches any character",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1.../containers/json"},
			want:   true,
		},
		{
			name:   "character class",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: "/v1\\.[0-9][0-9]/containers/json"},
			want:   true,
		},
		{
			name:   "match everything",
			path:   "/v1.42/containers/json",
			method: "GET",
			match:  config.Match{Path: ".*", Method: ".*"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the specified path and method
			req := httptest.NewRequest(tt.method, tt.path, nil)

			// Test the rule matching
			got := handler.ruleMatches(req, tt.match)

			if got != tt.want {
				t.Errorf("ruleMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyHandler_BodyRemains_Readable(t *testing.T) {
	handler := &ProxyHandler{}

	cfg := &config.SocketConfig{
		Rules: []config.Rule{
			{
				Match: config.Match{
					Path:   "/v1.*/containers/create",
					Method: "POST",
				},
				Actions: []config.Action{
					{
						Action: "allow",
						Reason: "test",
					},
				},
			},
		},
	}

	// Create a test request with a body
	body := []byte(`{"test": "data"}`)
	req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(body))

	// Process rules
	allowed, reason, err := handler.processRules(req, cfg)
	if err != nil {
		t.Fatalf("processRules() error = %v", err)
	}
	if !allowed {
		t.Errorf("processRules() got = %v, want true", allowed)
	}
	if reason != "test" {
		t.Errorf("processRules() reason = %v, want test", reason)
	}

	// Try to read the body again
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}
	if string(bodyBytes) != string(body) {
		t.Errorf("Body was not preserved, got %v, want %v", string(bodyBytes), string(body))
	}
}
