package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"docker-socket-proxy/internal/proxy/config"
)

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
