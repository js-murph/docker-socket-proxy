package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"docker-socket-proxy/internal/proxy/config"
)

func TestManagementHandler_CreateSocket(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{})
	defer handler.Cleanup()

	tests := []struct {
		name       string
		config     *config.SocketConfig
		wantStatus int
	}{
		{
			name: "valid config",
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
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty config",
			config:     nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.config != nil {
				body, _ = json.Marshal(tt.config)
			}

			req := httptest.NewRequest("POST", "/create-socket", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				socketPath := strings.TrimSpace(w.Body.String())
				if socketPath == "" {
					t.Error("expected socket path in response")
				}

				// Wait for the socket to be ready
				for i := 0; i < 10; i++ {
					if _, err := os.Stat(socketPath); err == nil {
						break
					}
					time.Sleep(50 * time.Millisecond)
				}

				// Verify the socket exists
				if _, err := os.Stat(socketPath); err != nil {
					t.Errorf("socket file not created: %v", err)
				}
			}
		})
	}
}

func TestManagementHandler_DeleteSocket(t *testing.T) {
	configs := make(map[string]*config.SocketConfig)
	handler := NewManagementHandler("/tmp/docker.sock", configs, &sync.RWMutex{})

	tests := []struct {
		name       string
		socketPath string
		wantStatus int
	}{
		{
			name:       "missing socket path",
			socketPath: "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid delete request",
			socketPath: "/tmp/test.sock",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/delete-socket", nil)
			if tt.socketPath != "" {
				req.Header.Set("Socket-Path", tt.socketPath)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}
