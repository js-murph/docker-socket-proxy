package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
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

			got, reason := handler.checkACLRules(tt.socketPath, tt.request)
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
