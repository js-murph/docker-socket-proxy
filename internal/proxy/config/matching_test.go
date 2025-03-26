package config

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

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
			r := &http.Request{
				Method: tt.method,
				URL: &url.URL{
					Path: tt.path,
				},
			}

			match := Match{
				Path:   tt.rule.path,
				Method: tt.rule.method,
			}

			if got := MatchesRule(r, match); got != tt.want {
				t.Errorf("MatchesRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsMatching(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
		match   Match
		want    bool
	}{
		{
			name:    "match path and method",
			request: httptest.NewRequest("GET", "/v1.24/containers/json", nil),
			match: Match{
				Path:   "/v1.*/containers/json",
				Method: "GET",
			},
			want: true,
		},
		{
			name:    "match path only",
			request: httptest.NewRequest("GET", "/v1.24/containers/json", nil),
			match: Match{
				Path: "/v1.*/containers/json",
			},
			want: true,
		},
		{
			name:    "match method only",
			request: httptest.NewRequest("GET", "/v1.24/containers/json", nil),
			match: Match{
				Method: "GET",
			},
			want: true,
		},
		{
			name:    "no match path",
			request: httptest.NewRequest("GET", "/v1.24/containers/json", nil),
			match: Match{
				Path:   "/v1.*/images/json",
				Method: "GET",
			},
			want: false,
		},
		{
			name:    "no match method",
			request: httptest.NewRequest("GET", "/v1.24/containers/json", nil),
			match: Match{
				Path:   "/v1.*/containers/json",
				Method: "POST",
			},
			want: false,
		},
		{
			name: "match simple env variable",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
				},
			},
			want: true,
		},
		{
			name: "match partial env variable",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Env": []interface{}{"DEBUG=true", "APP=test"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG.*"},
				},
			},
			want: true,
		},
		{
			name: "match nested field",
			request: func() *http.Request {
				body := map[string]interface{}{
					"HostConfig": map[string]interface{}{
						"Privileged": true,
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"HostConfig": map[string]interface{}{
						"Privileged": true,
					},
				},
			},
			want: true,
		},
		{
			name: "no match env variable",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Env": []interface{}{"APP=test"},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
				},
			},
			want: false,
		},
		{
			name: "match labels",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Labels": map[string]interface{}{
						"com.example.label1": "value1",
						"com.example.label2": "value2",
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Labels": map[string]interface{}{
						"com.example.label1": "value1",
					},
				},
			},
			want: true,
		},
		{
			name: "match volume mounts",
			request: func() *http.Request {
				body := map[string]interface{}{
					"HostConfig": map[string]interface{}{
						"Binds": []interface{}{
							"/host/path:/container/path",
							"/another/path:/another/container/path",
						},
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"HostConfig": map[string]interface{}{
						"Binds": []interface{}{"/host/path:/container/path"},
					},
				},
			},
			want: true,
		},
		{
			name: "match multiple conditions",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Env": []interface{}{"DEBUG=true", "APP=test"},
					"HostConfig": map[string]interface{}{
						"Privileged": true,
						"Binds": []interface{}{
							"/host/path:/container/path",
						},
					},
					"Labels": map[string]interface{}{
						"com.example.label1": "value1",
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
					"HostConfig": map[string]interface{}{
						"Privileged": true,
					},
					"Labels": map[string]interface{}{
						"com.example.label1": "value1",
					},
				},
			},
			want: true,
		},
		{
			name: "no match multiple conditions",
			request: func() *http.Request {
				body := map[string]interface{}{
					"Env": []interface{}{"DEBUG=true", "APP=test"},
					"HostConfig": map[string]interface{}{
						"Privileged": false,
					},
				}
				bodyBytes, _ := json.Marshal(body)
				req := httptest.NewRequest("POST", "/v1.24/containers/create", bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
				return req
			}(),
			match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
					"HostConfig": map[string]interface{}{
						"Privileged": true,
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesRule(tt.request, tt.match)
			if got != tt.want {
				t.Errorf("MatchesRule() = %v, want %v", got, tt.want)
			}
		})
	}
}
