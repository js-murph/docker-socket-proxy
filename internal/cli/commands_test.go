package cli

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"docker-socket-proxy/internal/management"

	"github.com/spf13/cobra"
)

func TestRunCreate(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test config file
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
rules:
  acls:
    - match:
        path: "/v1.*/containers"
        method: "GET"
      action: "allow"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a mock Unix socket server
	socketPath := filepath.Join(tmpDir, "test.sock")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Create a test server
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/create-socket" {
			t.Errorf("Expected /create-socket path, got %s", r.URL.Path)
		}
		w.Write([]byte("/var/run/test-socket.sock"))
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command
	cmd := &cobra.Command{}
	cmd.Flags().String("config", configPath, "")

	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Run the command
	RunCreate(cmd, paths)
}

func TestRunDelete(t *testing.T) {
	// Create a temporary directory for the test socket
	tmpDir, err := os.MkdirTemp("", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock Unix socket server
	socketPath := filepath.Join(tmpDir, "test.sock")
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Create a test server
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/delete-socket" {
			t.Errorf("Expected /delete-socket path, got %s", r.URL.Path)
		}
		if r.Header.Get("Socket-Path") != "/var/run/test-socket.sock" {
			t.Errorf("Expected Socket-Path header to be /var/run/test-socket.sock, got %s",
				r.Header.Get("Socket-Path"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	args := []string{"/var/run/test-socket.sock"}
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Run the command
	RunDelete(cmd, args, paths)
}
