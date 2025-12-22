package application

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"testing"
)

func TestIntegration_SocketService_GivenValidConfig_WhenCreatingSocket_ThenSucceeds(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	// Act
	socket, err := service.CreateSocket(context.Background(), config)

	// Assert
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}
	if socket.Path == "" {
		t.Errorf("CreateSocket() socket.Path = empty, want non-empty")
	}
	if len(socket.Config.Rules) != 1 {
		t.Errorf("CreateSocket() socket.Config.Rules = %d, want 1", len(socket.Config.Rules))
	}
}

func TestIntegration_SocketService_GivenExistingSocket_WhenDeletingSocket_ThenSucceeds(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	// Create a socket first
	socket, err := service.CreateSocket(context.Background(), config)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	// Act
	err = service.DeleteSocket(context.Background(), socket.Config.Name)

	// Assert
	if err != nil {
		t.Fatalf("DeleteSocket() error = %v, want nil", err)
	}
}

func TestIntegration_SocketService_GivenNonExistentSocket_WhenDeletingSocket_ThenReturnsError(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	// Act
	err := service.DeleteSocket(context.Background(), "/non/existent/socket.sock")

	// Assert
	if err == nil {
		t.Errorf("DeleteSocket() error = nil, want error")
	}
}

func TestIntegration_SocketService_GivenMultipleSockets_WhenListingSockets_ThenReturnsAll(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	config1 := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	config2 := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/images/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	// Create two sockets
	_, err := service.CreateSocket(context.Background(), config1)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	_, err = service.CreateSocket(context.Background(), config2)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	// Act
	sockets, err := service.ListSockets(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("ListSockets() error = %v, want nil", err)
	}
	if len(sockets) != 2 {
		t.Errorf("ListSockets() len = %d, want 2", len(sockets))
	}
}

func TestIntegration_SocketService_GivenExistingSocket_WhenDescribingSocket_ThenReturnsConfig(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	config := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	// Create a socket
	socket, err := service.CreateSocket(context.Background(), config)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	socketName := socket.Config.Name // Use the socket name

	// Act
	returnedConfig, err := service.DescribeSocket(context.Background(), socketName)

	// Assert
	if err != nil {
		t.Fatalf("DescribeSocket() error = %v, want nil", err)
	}
	if len(returnedConfig.Rules) != 1 {
		t.Errorf("DescribeSocket() config.Rules = %d, want 1", len(returnedConfig.Rules))
	}
}

func TestIntegration_SocketService_GivenNonExistentSocket_WhenDescribingSocket_ThenReturnsError(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	// Act
	_, err := service.DescribeSocket(context.Background(), "non-existent")

	// Assert
	if err == nil {
		t.Errorf("DescribeSocket() error = nil, want error")
	}
}

func TestIntegration_SocketService_GivenMultipleSockets_WhenCleaningSockets_ThenRemovesAll(t *testing.T) {
	// Arrange
	repo := NewInMemorySocketRepository()
	manager := NewInMemorySocketManager()
	service := NewSocketService(repo, manager)

	config1 := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/containers/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	config2 := domain.SocketConfig{
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.*/images/json",
					Method: "GET",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow},
				},
			},
		},
	}

	// Create two sockets
	_, err := service.CreateSocket(context.Background(), config1)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	_, err = service.CreateSocket(context.Background(), config2)
	if err != nil {
		t.Fatalf("CreateSocket() error = %v, want nil", err)
	}

	// Act
	err = service.CleanSockets(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("CleanSockets() error = %v, want nil", err)
	}

	// Verify all sockets are removed
	sockets, err := service.ListSockets(context.Background())
	if err != nil {
		t.Fatalf("ListSockets() error = %v, want nil", err)
	}
	if len(sockets) != 0 {
		t.Errorf("ListSockets() len = %d, want 0", len(sockets))
	}
}
