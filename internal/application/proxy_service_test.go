package application

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockDockerClient implements DockerClient for testing
type mockDockerClient struct {
	responses map[string]*http.Response
}

func (m *mockDockerClient) Do(req *http.Request) (*http.Response, error) {
	// Return a mock response based on the request path
	response, exists := m.responses[req.URL.Path]
	if !exists {
		response = &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       http.NoBody,
		}
	}
	return response, nil
}

func TestIntegration_ProxyService_GivenAllowedRequest_WhenProcessing_ThenForwardsToDocker(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{
		responses: map[string]*http.Response{
			"/v1.42/containers/json": {
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       http.NoBody,
			},
		},
	}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)

	// Act
	resp, err := service.ProcessRequest(context.Background(), req, "/test/socket.sock")

	// Assert
	if err != nil {
		t.Fatalf("ProcessRequest() error = %v, want nil", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ProcessRequest() status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestIntegration_ProxyService_GivenDeniedRequest_WhenProcessing_ThenReturnsForbidden(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("POST", "/v1.42/containers/create", nil)

	// Create a config that denies this request
	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/create",
					Method: "POST",
				},
				Actions: []domain.Action{
					{Type: domain.ActionDeny, Reason: "Not allowed"},
				},
			},
		},
	}

	// Act
	result, err := service.EvaluateRules(context.Background(), domain.NewRequest(req), config)

	// Assert
	if err != nil {
		t.Fatalf("EvaluateRules() error = %v, want nil", err)
	}
	if result.Allowed {
		t.Errorf("EvaluateRules() allowed = %v, want false", result.Allowed)
	}
	if result.Reason != "Not allowed" {
		t.Errorf("EvaluateRules() reason = %v, want 'Not allowed'", result.Reason)
	}
}

func TestIntegration_ProxyService_GivenAllowedRequest_WhenEvaluating_ThenReturnsAllowed(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)

	// Create a config that allows this request
	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow, Reason: "Allowed"},
				},
			},
		},
	}

	// Act
	result, err := service.EvaluateRules(context.Background(), domain.NewRequest(req), config)

	// Assert
	if err != nil {
		t.Fatalf("EvaluateRules() error = %v, want nil", err)
	}
	if !result.Allowed {
		t.Errorf("EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if result.Reason != "Allowed" {
		t.Errorf("EvaluateRules() reason = %v, want 'Allowed'", result.Reason)
	}
}

func TestIntegration_ProxyService_GivenNoRules_WhenEvaluating_ThenAllowsByDefault(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)

	// Create a config with no rules
	config := domain.SocketConfig{
		Rules: []domain.Rule{},
	}

	// Act
	result, err := service.EvaluateRules(context.Background(), domain.NewRequest(req), config)

	// Assert
	if err != nil {
		t.Fatalf("EvaluateRules() error = %v, want nil", err)
	}
	if !result.Allowed {
		t.Errorf("EvaluateRules() allowed = %v, want true", result.Allowed)
	}
}

func TestIntegration_ProxyService_GivenNonMatchingRules_WhenEvaluating_ThenAllowsByDefault(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("GET", "/v1.42/containers/json", nil)

	// Create a config with rules that don't match
	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/images/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionDeny, Reason: "Not allowed"},
				},
			},
		},
	}

	// Act
	result, err := service.EvaluateRules(context.Background(), domain.NewRequest(req), config)

	// Assert
	if err != nil {
		t.Fatalf("EvaluateRules() error = %v, want nil", err)
	}
	if !result.Allowed {
		t.Errorf("EvaluateRules() allowed = %v, want true", result.Allowed)
	}
}

func TestIntegration_ProxyService_GivenMultipleRules_WhenEvaluating_ThenFirstMatchWins(t *testing.T) {
	// Arrange
	mockClient := &mockDockerClient{}
	service := NewProxyService(mockClient, &mockSocketManager{})

	req := httptest.NewRequest("POST", "/v1.42/containers/create", nil)

	// Create a config with multiple rules
	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/create",
					Method: "POST",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow, Reason: "First rule allows"},
				},
			},
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/create",
					Method: "POST",
				},
				Actions: []domain.Action{
					{Type: domain.ActionDeny, Reason: "Second rule denies"},
				},
			},
		},
	}

	// Act
	result, err := service.EvaluateRules(context.Background(), domain.NewRequest(req), config)

	// Assert
	if err != nil {
		t.Fatalf("EvaluateRules() error = %v, want nil", err)
	}
	if !result.Allowed {
		t.Errorf("EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if result.Reason != "First rule allows" {
		t.Errorf("EvaluateRules() reason = %v, want 'First rule allows'", result.Reason)
	}
}

// mockSocketManager implements SocketManager for testing
type mockSocketManager struct{}

func (m *mockSocketManager) CreateSocket(name string, config domain.SocketConfig) error {
	return nil
}

func (m *mockSocketManager) DeleteSocket(name string) error {
	return nil
}

func (m *mockSocketManager) ListSockets() []string {
	return []string{}
}

func (m *mockSocketManager) GetSocket(name string) (domain.SocketConfig, bool) {
	return domain.SocketConfig{}, false
}

func (m *mockSocketManager) GetSocketDir() string {
	return "/tmp/sockets"
}
