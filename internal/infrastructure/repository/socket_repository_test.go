package repository

import (
	"context"
	"docker-socket-proxy/internal/domain"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemorySocketRepository_SaveAndLoad(t *testing.T) {
	repo := NewInMemorySocketRepository()
	ctx := context.Background()

	config := domain.SocketConfig{
		Name:            "test-socket",
		ListenAddress:   "/tmp/test-socket.sock",
		DockerDaemonURL: "unix:///var/run/docker.sock",
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.42/containers/create",
					Method: "POST",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow, Reason: "Allowed to create containers"},
				},
			},
		},
	}

	// Test Save
	err := repo.Save(ctx, "test-socket", config)
	if err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	// Test Load
	found, err := repo.Load(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if found.Name != config.Name {
		t.Errorf("Load() got name = %v, want %v", found.Name, config.Name)
	}
	if found.ListenAddress != config.ListenAddress {
		t.Errorf("Load() got listen address = %v, want %v", found.ListenAddress, config.ListenAddress)
	}
	if found.DockerDaemonURL != config.DockerDaemonURL {
		t.Errorf("Load() got docker daemon URL = %v, want %v", found.DockerDaemonURL, config.DockerDaemonURL)
	}
	if len(found.Rules) != len(config.Rules) {
		t.Errorf("Load() got %d rules, want %d", len(found.Rules), len(config.Rules))
	}
}

func TestInMemorySocketRepository_Load_NotFound(t *testing.T) {
	repo := NewInMemorySocketRepository()
	ctx := context.Background()

	_, err := repo.Load(ctx, "non-existent")
	if err == nil {
		t.Errorf("Load() error = nil, want error")
	}
}

func TestInMemorySocketRepository_List(t *testing.T) {
	repo := NewInMemorySocketRepository()
	ctx := context.Background()

	config1 := domain.SocketConfig{Name: "socket1", ListenAddress: "/tmp/socket1.sock"}
	config2 := domain.SocketConfig{Name: "socket2", ListenAddress: "/tmp/socket2.sock"}

	require.NoError(t, repo.Save(ctx, "socket1", config1))
	require.NoError(t, repo.Save(ctx, "socket2", config2))

	configs, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(configs) != 2 {
		t.Errorf("List() got %d configs, want 2", len(configs))
	}
}

func TestInMemorySocketRepository_Delete(t *testing.T) {
	repo := NewInMemorySocketRepository()
	ctx := context.Background()

	config := domain.SocketConfig{Name: "test-socket", ListenAddress: "/tmp/test-socket.sock"}
	require.NoError(t, repo.Save(ctx, "test-socket", config))

	// Verify it exists
	_, err := repo.Load(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Load() before delete error = %v, want nil", err)
	}

	// Delete it
	err = repo.Delete(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	_, err = repo.Load(ctx, "test-socket")
	if err == nil {
		t.Errorf("Load() after delete error = nil, want error")
	}
}

func TestFileSocketRepository_SaveAndLoad(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "socket-repo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFileSocketRepository(tempDir)
	ctx := context.Background()

	config := domain.SocketConfig{
		Name:            "test-socket",
		ListenAddress:   "/tmp/test-socket.sock",
		DockerDaemonURL: "unix:///var/run/docker.sock",
		Rules: []domain.Rule{
			{
				Match: domain.Match{
					Path:   "/v1.42/containers/create",
					Method: "POST",
				},
				Actions: []domain.Action{
					{Type: domain.ActionAllow, Reason: "Allowed to create containers"},
				},
			},
		},
	}

	// Test Save
	err = repo.Save(ctx, "test-socket", config)
	if err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	// Test Load
	found, err := repo.Load(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	// The loaded config should preserve the original values
	if found.Name != config.Name {
		t.Errorf("Load() got name = %v, want %v", found.Name, config.Name)
	}
	if found.ListenAddress != config.ListenAddress {
		t.Errorf("Load() got listen address = %v, want %v", found.ListenAddress, config.ListenAddress)
	}
	if found.DockerDaemonURL != config.DockerDaemonURL {
		t.Errorf("Load() got docker daemon URL = %v, want %v", found.DockerDaemonURL, config.DockerDaemonURL)
	}
	if len(found.Rules) != len(config.Rules) {
		t.Errorf("Load() got %d rules, want %d", len(found.Rules), len(config.Rules))
	}
}

func TestFileSocketRepository_Load_NotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "socket-repo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFileSocketRepository(tempDir)
	ctx := context.Background()

	_, err = repo.Load(ctx, "non-existent")
	if err == nil {
		t.Errorf("Load() error = nil, want error")
	}
}

func TestFileSocketRepository_List(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "socket-repo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFileSocketRepository(tempDir)
	ctx := context.Background()

	config1 := domain.SocketConfig{Name: "/tmp/socket1.sock", ListenAddress: "/tmp/socket1.sock"}
	config2 := domain.SocketConfig{Name: "/tmp/socket2.sock", ListenAddress: "/tmp/socket2.sock"}

	require.NoError(t, repo.Save(ctx, "/tmp/socket1.sock", config1))
	require.NoError(t, repo.Save(ctx, "/tmp/socket2.sock", config2))

	configs, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(configs) != 2 {
		t.Errorf("List() got %d configs, want 2", len(configs))
	}
}

func TestFileSocketRepository_Delete(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "socket-repo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFileSocketRepository(tempDir)
	ctx := context.Background()

	config := domain.SocketConfig{Name: "test-socket", ListenAddress: "/tmp/test-socket.sock"}
	require.NoError(t, repo.Save(ctx, "test-socket", config))

	// Verify it exists
	_, err = repo.Load(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Load() before delete error = %v, want nil", err)
	}

	// Delete it
	err = repo.Delete(ctx, "test-socket")
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	_, err = repo.Load(ctx, "test-socket")
	if err == nil {
		t.Errorf("Load() after delete error = nil, want error")
	}
}

func TestFileSocketRepository_ConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "socket-repo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFileSocketRepository(tempDir)
	ctx := context.Background()

	// Test concurrent saves
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			socketPath := fmt.Sprintf("/tmp/concurrent-socket-%d.sock", i)
			config := domain.SocketConfig{
				Name:            socketPath,
				ListenAddress:   socketPath,
				DockerDaemonURL: "unix:///var/run/docker.sock",
			}
			err := repo.Save(ctx, socketPath, config)
			if err != nil {
				t.Errorf("Concurrent Save() error = %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all configs were saved
	configs, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(configs) != 10 {
		t.Errorf("List() got %d configs, want 10", len(configs))
	}
}
