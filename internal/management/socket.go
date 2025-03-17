package management

// Constants for socket paths
const (
	DefaultManagementSocketPath = "/var/run/docker-proxy-mgmt.sock"
	DefaultDockerSocketPath     = "/var/run/docker.sock"
)

// SocketPaths holds the configured paths for various sockets
type SocketPaths struct {
	Management string
	Docker     string
}

// NewSocketPaths creates a new SocketPaths with default values
func NewSocketPaths() *SocketPaths {
	return &SocketPaths{
		Management: DefaultManagementSocketPath,
		Docker:     DefaultDockerSocketPath,
	}
}
