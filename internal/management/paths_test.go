package management

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSocketPaths(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

	// Test with default paths
	paths := NewSocketPaths()
	if paths.Management != DefaultManagementSocketPath {
		t.Errorf("Expected default management path, got %s", paths.Management)
	}
	if paths.Docker != DefaultDockerSocketPath {
		t.Errorf("Expected default docker path, got %s", paths.Docker)
	}
	if paths.SocketDir != DefaultSocketDir {
		t.Errorf("Expected default socket dir, got %s", paths.SocketDir)
	}
}

func TestSocketPathsWithCustomPaths(t *testing.T) {
	// Create temporary directory for test sockets
	tmpDir, err := os.MkdirTemp("/tmp", "socket-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	customPaths := &SocketPaths{
		Management: filepath.Join(tmpDir, "mgmt.sock"),
		Docker:     filepath.Join(tmpDir, "docker.sock"),
	}

	if customPaths.Management == DefaultManagementSocketPath {
		t.Error("Custom management path should be different from default")
	}

	if customPaths.Docker == DefaultDockerSocketPath {
		t.Error("Custom docker path should be different from default")
	}
}

func TestSocketPathsValidation(t *testing.T) {
	tests := []struct {
		name    string
		paths   *SocketPaths
		wantErr bool
	}{
		{
			name: "valid paths",
			paths: &SocketPaths{
				Management: "/tmp/mgmt.sock",
				Docker:     "/tmp/docker.sock",
			},
			wantErr: false,
		},
		{
			name: "empty management path",
			paths: &SocketPaths{
				Management: "",
				Docker:     "/tmp/docker.sock",
			},
			wantErr: true,
		},
		{
			name: "empty docker path",
			paths: &SocketPaths{
				Management: "/tmp/mgmt.sock",
				Docker:     "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSocketPaths(tt.paths)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSocketPaths() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function for validation
func validateSocketPaths(paths *SocketPaths) error {
	if paths.Management == "" {
		return fmt.Errorf("management socket path cannot be empty")
	}
	if paths.Docker == "" {
		return fmt.Errorf("docker socket path cannot be empty")
	}
	return nil
}
