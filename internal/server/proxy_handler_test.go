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
		w.Write([]byte(`{"message":"OK"}`))
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
	socketConfig, _ := h.socketConfigs[socketPath]
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
	io.Copy(w, resp.Body)
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
