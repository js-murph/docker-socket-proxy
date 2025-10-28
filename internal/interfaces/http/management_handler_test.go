package http

import (
	"bytes"
	"context"
	"docker-socket-proxy/internal/domain"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// MockSocketService is a mock implementation of SocketService for testing
type MockSocketService struct {
	createSocketFunc   func(ctx context.Context, config domain.SocketConfig) (domain.Socket, error)
	deleteSocketFunc   func(ctx context.Context, socketPath string) error
	listSocketsFunc    func(ctx context.Context) ([]string, error)
	describeSocketFunc func(ctx context.Context, socketName string) (domain.SocketConfig, error)
	cleanSocketsFunc   func(ctx context.Context) error
}

func (m *MockSocketService) CreateSocket(ctx context.Context, config domain.SocketConfig) (domain.Socket, error) {
	if m.createSocketFunc != nil {
		return m.createSocketFunc(ctx, config)
	}
	return domain.Socket{}, nil
}

func (m *MockSocketService) DeleteSocket(ctx context.Context, socketPath string) error {
	if m.deleteSocketFunc != nil {
		return m.deleteSocketFunc(ctx, socketPath)
	}
	return nil
}

func (m *MockSocketService) ListSockets(ctx context.Context) ([]string, error) {
	if m.listSocketsFunc != nil {
		return m.listSocketsFunc(ctx)
	}
	return []string{}, nil
}

func (m *MockSocketService) DescribeSocket(ctx context.Context, socketName string) (domain.SocketConfig, error) {
	if m.describeSocketFunc != nil {
		return m.describeSocketFunc(ctx, socketName)
	}
	return domain.SocketConfig{}, nil
}

func (m *MockSocketService) CleanSockets(ctx context.Context) error {
	if m.cleanSocketsFunc != nil {
		return m.cleanSocketsFunc(ctx)
	}
	return nil
}

func TestManagementHandler_CreateSocketHandler(t *testing.T) {
	tests := []struct {
		name           string
		request        CreateSocketRequest
		mockFunc       func(ctx context.Context, config domain.SocketConfig) (domain.Socket, error)
		expectedStatus int
		expectedError  bool
	}{
		{
			name: "successful creation",
			request: CreateSocketRequest{
				Name:            "test-socket",
				ListenAddress:   "/tmp/test-socket.sock",
				DockerDaemonURL: "unix:///var/run/docker.sock",
				Rules: []RuleRequest{
					{
						Match: MatchRequest{
							Path:   "/v1.42/containers/create",
							Method: "POST",
						},
						Actions: []ActionRequest{
							{Action: "allow", Reason: "Allowed to create containers"},
						},
					},
				},
			},
			mockFunc: func(ctx context.Context, config domain.SocketConfig) (domain.Socket, error) {
				return domain.Socket{
					Path:   config.ListenAddress,
					Config: config,
				}, nil
			},
			expectedStatus: http.StatusCreated,
			expectedError:  false,
		},
		{
			name: "service error",
			request: CreateSocketRequest{
				Name:            "test-socket",
				ListenAddress:   "/tmp/test-socket.sock",
				DockerDaemonURL: "unix:///var/run/docker.sock",
				Rules:           []RuleRequest{},
			},
			mockFunc: func(ctx context.Context, config domain.SocketConfig) (domain.Socket, error) {
				return domain.Socket{}, fmt.Errorf("service error")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := &MockSocketService{
				createSocketFunc: tt.mockFunc,
			}

			// Create handler
			handler := NewManagementHandler(mockService)

			// Create request
			reqBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/socket/create", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.CreateSocketHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("CreateSocketHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			// Check response body for errors
			if tt.expectedError {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if response["success"] != nil {
					t.Errorf("CreateSocketHandler() expected error response, got success")
				}
			} else {
				var response CreateSocketResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if !response.Success {
					t.Errorf("CreateSocketHandler() expected success response, got error")
				}
			}
		})
	}
}

func TestManagementHandler_ListSocketsHandler(t *testing.T) {
	tests := []struct {
		name           string
		mockFunc       func(ctx context.Context) ([]string, error)
		expectedStatus int
		expectedError  bool
	}{
		{
			name: "successful list",
			mockFunc: func(ctx context.Context) ([]string, error) {
				return []string{"/tmp/socket1.sock", "/tmp/socket2.sock"}, nil
			},
			expectedStatus: http.StatusOK,
			expectedError:  false,
		},
		{
			name: "service error",
			mockFunc: func(ctx context.Context) ([]string, error) {
				return nil, fmt.Errorf("service error")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := &MockSocketService{
				listSocketsFunc: tt.mockFunc,
			}

			// Create handler
			handler := NewManagementHandler(mockService)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/socket/list", nil)

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.ListSocketsHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("ListSocketsHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			// Check response body for errors
			if tt.expectedError {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if response["success"] != nil {
					t.Errorf("ListSocketsHandler() expected error response, got success")
				}
			} else {
				var response ListSocketsResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if !response.Success {
					t.Errorf("ListSocketsHandler() expected success response, got error")
				}
			}
		})
	}
}

func TestManagementHandler_DescribeSocketHandler(t *testing.T) {
	tests := []struct {
		name           string
		socketName     string
		mockFunc       func(ctx context.Context, socketName string) (domain.SocketConfig, error)
		expectedStatus int
		expectedError  bool
	}{
		{
			name:       "successful describe",
			socketName: "test-socket",
			mockFunc: func(ctx context.Context, socketName string) (domain.SocketConfig, error) {
				return domain.SocketConfig{
					Name:            socketName,
					ListenAddress:   "/tmp/test-socket.sock",
					DockerDaemonURL: "unix:///var/run/docker.sock",
				}, nil
			},
			expectedStatus: http.StatusOK,
			expectedError:  false,
		},
		{
			name:       "socket not found",
			socketName: "nonexistent-socket",
			mockFunc: func(ctx context.Context, socketName string) (domain.SocketConfig, error) {
				return domain.SocketConfig{}, fmt.Errorf("socket not found")
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  true,
		},
		{
			name:           "missing socket parameter",
			socketName:     "",
			mockFunc:       nil,
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := &MockSocketService{
				describeSocketFunc: tt.mockFunc,
			}

			// Create handler
			handler := NewManagementHandler(mockService)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/socket/describe?socket="+tt.socketName, nil)

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.DescribeSocketHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("DescribeSocketHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			// Check response body for errors
			if tt.expectedError {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if response["success"] != nil {
					t.Errorf("DescribeSocketHandler() expected error response, got success")
				}
			} else {
				var response DescribeSocketResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if !response.Success {
					t.Errorf("DescribeSocketHandler() expected success response, got error")
				}
			}
		})
	}
}

func TestManagementHandler_DeleteSocketHandler(t *testing.T) {
	tests := []struct {
		name           string
		socketName     string
		mockFunc       func(ctx context.Context, socketPath string) error
		expectedStatus int
		expectedError  bool
	}{
		{
			name:       "successful delete",
			socketName: "test-socket",
			mockFunc: func(ctx context.Context, socketPath string) error {
				return nil
			},
			expectedStatus: http.StatusOK,
			expectedError:  false,
		},
		{
			name:       "service error",
			socketName: "test-socket",
			mockFunc: func(ctx context.Context, socketPath string) error {
				return fmt.Errorf("service error")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  true,
		},
		{
			name:           "missing socket parameter",
			socketName:     "",
			mockFunc:       nil,
			expectedStatus: http.StatusBadRequest,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := &MockSocketService{
				deleteSocketFunc: tt.mockFunc,
			}

			// Create handler
			handler := NewManagementHandler(mockService)

			// Create request
			req := httptest.NewRequest(http.MethodDelete, "/socket/delete?socket="+tt.socketName, nil)

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.DeleteSocketHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("DeleteSocketHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			// Check response body for errors
			if tt.expectedError {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if response["success"] != nil {
					t.Errorf("DeleteSocketHandler() expected error response, got success")
				}
			} else {
				var response DeleteSocketResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if !response.Success {
					t.Errorf("DeleteSocketHandler() expected success response, got error")
				}
			}
		})
	}
}

func TestManagementHandler_CleanSocketsHandler(t *testing.T) {
	tests := []struct {
		name           string
		mockFunc       func(ctx context.Context) error
		expectedStatus int
		expectedError  bool
	}{
		{
			name: "successful clean",
			mockFunc: func(ctx context.Context) error {
				return nil
			},
			expectedStatus: http.StatusOK,
			expectedError:  false,
		},
		{
			name: "service error",
			mockFunc: func(ctx context.Context) error {
				return fmt.Errorf("service error")
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := &MockSocketService{
				cleanSocketsFunc: tt.mockFunc,
			}

			// Create handler
			handler := NewManagementHandler(mockService)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/socket/clean", nil)

			// Create response recorder
			w := httptest.NewRecorder()

			// Call handler
			handler.CleanSocketsHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("CleanSocketsHandler() status = %v, want %v", w.Code, tt.expectedStatus)
			}

			// Check response body for errors
			if tt.expectedError {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if response["success"] != nil {
					t.Errorf("CleanSocketsHandler() expected error response, got success")
				}
			} else {
				var response CleanSocketsResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				if !response.Success {
					t.Errorf("CleanSocketsHandler() expected success response, got error")
				}
			}
		})
	}
}

func TestManagementHandler_HealthHandler(t *testing.T) {
	// Create mock service
	mockService := &MockSocketService{}

	// Create handler
	handler := NewManagementHandler(mockService)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.HealthHandler(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("HealthHandler() status = %v, want %v", w.Code, http.StatusOK)
	}

	// Check response body
	var response HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	if response.Status != "healthy" {
		t.Errorf("HealthHandler() status = %v, want healthy", response.Status)
	}
}
