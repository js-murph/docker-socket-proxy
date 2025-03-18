package management

import (
	"fmt"
	"os"
)

const (
	DefaultManagementSocketPath = "/var/run/docker-proxy.sock"
	DefaultSocketDir            = "/var/run/docker-proxy/"
	DefaultDockerSocketPath     = "/var/run/docker.sock"
)

type SocketPaths struct {
	Management string
	Docker     string
	SocketDir  string // Directory for storing socket files
}

func NewSocketPaths() *SocketPaths {
	return &SocketPaths{
		Management: DefaultManagementSocketPath,
		Docker:     DefaultDockerSocketPath,
		SocketDir:  DefaultSocketDir,
	}
}

func (p *SocketPaths) Validate() error {
	if p.Management == "" {
		return fmt.Errorf("management socket path cannot be empty")
	}
	if p.Docker == "" {
		return fmt.Errorf("docker socket path cannot be empty")
	}
	if p.SocketDir == "" {
		return fmt.Errorf("socket directory cannot be empty")
	}

	// Create socket directory if it doesn't exist
	if err := os.MkdirAll(p.SocketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %v", err)
	}

	return nil
}

// GetManagementSocketPath returns the path to the management socket
func GetManagementSocketPath() (string, error) {
	// Default management socket path
	socketPath := DefaultManagementSocketPath

	// Check if a custom path is specified in the environment
	if envPath := os.Getenv("DOCKER_PROXY_MANAGEMENT_SOCKET"); envPath != "" {
		socketPath = envPath
	}

	return socketPath, nil
}
