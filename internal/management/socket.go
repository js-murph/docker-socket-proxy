package management

import "fmt"

// DefaultSocketPaths defines the standard Unix socket paths
const (
	// DefaultManagementSocketPath is the default path for the management socket
	// that handles proxy socket creation and deletion
	DefaultManagementSocketPath = "/var/run/docker-proxy-mgmt.sock"

	// DefaultDockerSocketPath is the default path to the Docker daemon socket
	DefaultDockerSocketPath = "/var/run/docker.sock"
)

// SocketPaths holds the paths for both the management and Docker daemon sockets.
// These paths can be configured via command-line flags.
type SocketPaths struct {
	// Management is the Unix socket path for the proxy management API
	Management string

	// Docker is the Unix socket path for the Docker daemon
	Docker string
}

// NewSocketPaths creates a new SocketPaths instance with default values.
// Use Validate() to ensure the paths are valid.
func NewSocketPaths() *SocketPaths {
	return &SocketPaths{
		Management: DefaultManagementSocketPath,
		Docker:     DefaultDockerSocketPath,
	}
}

// Validate ensures that both socket paths are properly configured.
// Returns an error if either path is empty.
func (p *SocketPaths) Validate() error {
	if p.Management == "" {
		return fmt.Errorf("management socket path cannot be empty")
	}
	if p.Docker == "" {
		return fmt.Errorf("docker socket path cannot be empty")
	}
	return nil
}
