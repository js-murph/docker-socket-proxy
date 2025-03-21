package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
	"io"
	"strings"
)

func TestProxyHandler_CheckACLRules(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	handler := NewProxyHandler("/tmp/docker.sock", configs, &sync.RWMutex{})

	tests := []struct {
		name       string
		socketPath string
		config     *config.SocketConfig
		request    *http.Request
		want       bool
		wantReason string
	}{
		{
			name:       "no config allows all",
			socketPath: "/tmp/test.sock",
			config:     nil,
			request:    httptest.NewRequest("GET", "/test", nil),
			want:       true,
			wantReason: "",
		},
		{
			name:       "explicit allow rule matches",
			socketPath: "/tmp/test.sock",
			config: &config.SocketConfig{
				Rules: config.RuleSet{
					ACLs: []config.Rule{
						{
							Match:  config.Match{Path: "/test", Method: "GET"},
							Action: "allow",
						},
					},
				},
			},
			request:    httptest.NewRequest("GET", "/test", nil),
			want:       true,
			wantReason: "",
		},
		{
			name:       "explicit deny rule matches",
			socketPath: "/tmp/test.sock",
			config: &config.SocketConfig{
				Rules: config.RuleSet{
					ACLs: []config.Rule{
						{
							Match:  config.Match{Path: "/test", Method: "POST"},
							Action: "deny",
							Reason: "method not allowed",
						},
					},
				},
			},
			request:    httptest.NewRequest("POST", "/test", nil),
			want:       false,
			wantReason: "method not allowed",
		},
		{
			name:       "no matching rules defaults to deny",
			socketPath: "/tmp/test.sock",
			config: &config.SocketConfig{
				Rules: config.RuleSet{
					ACLs: []config.Rule{
						{
							Match:  config.Match{Path: "/other", Method: "GET"},
							Action: "allow",
						},
					},
				},
			},
			request:    httptest.NewRequest("GET", "/test", nil),
			want:       false,
			wantReason: "No matching allow rules",
		},
		{
			name:       "first matching rule takes precedence",
			socketPath: "/tmp/test.sock",
			config: &config.SocketConfig{
				Rules: config.RuleSet{
					ACLs: []config.Rule{
						{
							Match:  config.Match{Path: "/test", Method: "GET"},
							Action: "allow",
						},
						{
							Match:  config.Match{Path: "/test", Method: "GET"},
							Action: "deny",
							Reason: "should not reach here",
						},
					},
				},
			},
			request:    httptest.NewRequest("GET", "/test", nil),
			want:       true,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config != nil {
				configs[tt.socketPath] = tt.config
			}

			got, reason := handler.checkACLs(tt.request, tt.config)
			if got != tt.want {
				t.Errorf("checkACLRules() got = %v, want %v", got, tt.want)
			}
			if reason != tt.wantReason {
				t.Errorf("checkACLRules() reason = %v, want %v", reason, tt.wantReason)
			}

			delete(configs, tt.socketPath)
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
		Rules: config.RuleSet{
			ACLs: []config.Rule{
				{
					Match:  config.Match{Path: "/v1.42/containers/json", Method: "GET"},
					Action: "allow",
				},
				{
					Match:  config.Match{Path: "/", Method: ""},
					Action: "deny",
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
	defer resp.Body.Close()

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

	// If there are no ACLs, deny by default
	if len(socketConfig.Rules.ACLs) == 0 {
		return false, "no ACLs defined"
	}

	// Check each ACL rule
	for _, rule := range socketConfig.Rules.ACLs {
		// Check if the rule matches the request
		if h.ruleMatches(r, rule.Match) {
			if rule.Action == "allow" {
				return true, ""
			} else {
				return false, "method not allowed"
			}
		}
	}

	// If no rule matches, deny by default
	return false, "No matching allow rules"
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

func TestContainsMatching(t *testing.T) {
	tests := []struct {
		name          string
		requestBody   string
		matchContains map[string]interface{}
		wantMatch     bool
	}{
		{
			name: "match simple env variable",
			requestBody: `{
				"Image": "nginx",
				"Env": ["DEBUG=true", "APP=test"]
			}`,
			matchContains: map[string]interface{}{
				"Env": "DEBUG=true",
			},
			wantMatch: true,
		},
		{
			name: "match env variable with array in config",
			requestBody: `{
				"Image": "nginx",
				"Env": ["DEBUG=true", "APP=test"]
			}`,
			matchContains: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true"},
			},
			wantMatch: true,
		},
		{
			name: "match multiple env variables",
			requestBody: `{
				"Image": "nginx",
				"Env": ["DEBUG=true", "APP=test", "BLOCK=true"]
			}`,
			matchContains: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "BLOCK=true"},
			},
			wantMatch: true,
		},
		{
			name: "no match when env variable not present",
			requestBody: `{
				"Image": "nginx",
				"Env": ["DEBUG=true", "APP=test"]
			}`,
			matchContains: map[string]interface{}{
				"Env": "BLOCK=true",
			},
			wantMatch: false,
		},
		{
			name: "match nested field",
			requestBody: `{
				"Image": "nginx",
				"HostConfig": {
					"Privileged": true,
					"Binds": ["/tmp:/tmp"]
				}
			}`,
			matchContains: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": true,
				},
			},
			wantMatch: true,
		},
		{
			name: "no match when nested field has different value",
			requestBody: `{
				"Image": "nginx",
				"HostConfig": {
					"Privileged": false,
					"Binds": ["/tmp:/tmp"]
				}
			}`,
			matchContains: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Privileged": true,
				},
			},
			wantMatch: false,
		},
		{
			name: "match array element in nested field",
			requestBody: `{
				"Image": "nginx",
				"HostConfig": {
					"Binds": ["/tmp:/tmp", "/var:/var"]
				}
			}`,
			matchContains: map[string]interface{}{
				"HostConfig": map[string]interface{}{
					"Binds": "/var:/var",
				},
			},
			wantMatch: true,
		},
		{
			name: "match partial string in env variable",
			requestBody: `{
				"Image": "nginx",
				"Env": ["DEBUG_LEVEL=verbose", "APP=test"]
			}`,
			matchContains: map[string]interface{}{
				"Env": "DEBUG",
			},
			wantMatch: true,
		},
		{
			name: "match when field exists but is empty",
			requestBody: `{
				"Image": "nginx",
				"Env": []
			}`,
			matchContains: map[string]interface{}{
				"Env": []interface{}{},
			},
			wantMatch: true,
		},
		{
			name: "complex nested structure",
			requestBody: `{
				"Image": "nginx",
				"Labels": {
					"com.example.vendor": "ACME",
					"com.example.version": "1.0"
				},
				"HostConfig": {
					"Devices": [
						{
							"PathOnHost": "/dev/deviceName",
							"PathInContainer": "/dev/deviceName",
							"CgroupPermissions": "rwm"
						}
					]
				}
			}`,
			matchContains: map[string]interface{}{
				"Labels": map[string]interface{}{
					"com.example.vendor": "ACME",
				},
			},
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the test body
			req := httptest.NewRequest("POST", "/v1.42/containers/create", strings.NewReader(tt.requestBody))

			// Create a match object with the test contains criteria
			match := config.Match{
				Path:     "/v1.42/containers/create",
				Method:   "POST",
				Contains: tt.matchContains,
			}

			// Create a handler to test
			handler := NewProxyHandler("/tmp/docker.sock", nil, &sync.RWMutex{})

			// Test the rule matching
			result := handler.ruleMatches(req, match)

			if result != tt.wantMatch {
				t.Errorf("ruleMatches() = %v, want %v", result, tt.wantMatch)
			}
		})
	}
}

func TestContainsValue(t *testing.T) {
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
		want     bool
	}{
		{
			name:     "string contains substring",
			actual:   "DEBUG=true",
			expected: "DEBUG",
			want:     true,
		},
		{
			name:     "string equals string",
			actual:   "DEBUG=true",
			expected: "DEBUG=true",
			want:     true,
		},
		{
			name:     "string does not contain substring",
			actual:   "APP=test",
			expected: "DEBUG",
			want:     false,
		},
		{
			name:     "array contains string",
			actual:   []interface{}{"DEBUG=true", "APP=test"},
			expected: "DEBUG=true",
			want:     true,
		},
		{
			name:     "array contains all strings in expected array",
			actual:   []interface{}{"DEBUG=true", "APP=test", "LEVEL=info"},
			expected: []interface{}{"DEBUG=true", "APP=test"},
			want:     true,
		},
		{
			name:     "array does not contain all strings in expected array",
			actual:   []interface{}{"DEBUG=true", "APP=test"},
			expected: []interface{}{"DEBUG=true", "LEVEL=info"},
			want:     false,
		},
		{
			name:     "boolean equals boolean",
			actual:   true,
			expected: true,
			want:     true,
		},
		{
			name:     "boolean does not equal boolean",
			actual:   true,
			expected: false,
			want:     false,
		},
		{
			name:     "nil equals nil",
			actual:   nil,
			expected: nil,
			want:     true,
		},
		{
			name:     "nil does not equal non-nil",
			actual:   nil,
			expected: "something",
			want:     false,
		},
		{
			name:     "array with partial string match",
			actual:   []interface{}{"DEBUG_LEVEL=verbose", "APP=test"},
			expected: "DEBUG",
			want:     true,
		},
		{
			name:     "regex match in string",
			actual:   "DEBUG_LEVEL=verbose",
			expected: "DEBUG.*verbose",
			want:     true,
		},
		{
			name:     "regex no match in string",
			actual:   "APP=test",
			expected: "DEBUG.*",
			want:     false,
		},
		{
			name:     "regex match in array",
			actual:   []interface{}{"DEBUG_LEVEL=verbose", "APP=test"},
			expected: "DEBUG.*verbose",
			want:     true,
		},
		{
			name:     "simple string still works",
			actual:   "DEBUG=true",
			expected: "DEBUG",
			want:     true,
		},
		{
			name:     "escaped regex characters treated as literal",
			actual:   "value with * asterisk",
			expected: "with \\* ast",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsValue(tt.actual, tt.expected)

			if got != tt.want {
				t.Errorf("containsValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
