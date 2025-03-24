package management

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSocketPaths(t *testing.T) {
	paths := NewSocketPaths()

	if paths.Management != DefaultManagementSocketPath {
		t.Errorf("Expected Management path to be %s, got %s",
			DefaultManagementSocketPath, paths.Management)
	}

	if paths.Docker != DefaultDockerSocketPath {
		t.Errorf("Expected Docker path to be %s, got %s",
			DefaultDockerSocketPath, paths.Docker)
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
