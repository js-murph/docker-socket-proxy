package config

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestApplyRewriteActions(t *testing.T) {
	tests := []struct {
		name         string
		body         map[string]interface{}
		actions      []Rule
		wantBody     map[string]interface{}
		wantModified bool
	}{
		{
			name: "replace env var",
			body: map[string]interface{}{
				"Env": []interface{}{"DEBUG=true", "OTHER=value"},
			},
			actions: []Rule{
				{
					Actions: []Action{
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
			actions: []Rule{
				{
					Actions: []Action{
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
			actions: []Rule{
				{
					Actions: []Action{
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
			actions: []Rule{
				{
					Actions: []Action{
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
						if MatchesStructure(body, action.Contains) {
							if MergeStructure(body, action.Update, true) {
								modified = true
							}
						}
					case "upsert":
						if MergeStructure(body, action.Update, false) {
							modified = true
						}
					case "delete":
						if DeleteMatchingFields(body, action.Contains) {
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

func createTestRequest(body map[string]interface{}) *http.Request {
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v1.42/containers/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestApplyRewriteRules(t *testing.T) {
	tests := []struct {
		name         string
		config       *SocketConfig
		request      *http.Request
		wantBody     map[string]interface{}
		wantModified bool
	}{
		{
			name: "apply multiple rewrites",
			config: &SocketConfig{
				Rules: []Rule{
					{
						Match: Match{
							Path:   "/v1.*/containers/create",
							Method: "POST",
						},
						Actions: []Action{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the request body
			bodyBytes, _ := io.ReadAll(tt.request.Body)
			tt.request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Parse the body
			var body map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				t.Fatalf("Failed to parse request body: %v", err)
			}

			// Apply rewrite rules
			modified := false
			for _, rule := range tt.config.Rules {
				if MatchesRule(tt.request, rule.Match) {
					for _, action := range rule.Actions {
						switch action.Action {
						case "replace":
							if MatchesStructure(body, action.Contains) {
								if MergeStructure(body, action.Update, true) {
									modified = true
								}
							}
						case "upsert":
							if MergeStructure(body, action.Update, false) {
								modified = true
							}
						}
					}
				}
			}

			if modified != tt.wantModified {
				t.Errorf("applyRewriteRules() modified = %v, want %v", modified, tt.wantModified)
			}

			if tt.wantModified {
				if !reflect.DeepEqual(body, tt.wantBody) {
					t.Errorf("body after rewrite = %v, want %v", body, tt.wantBody)
				}
			}
		})
	}
}
