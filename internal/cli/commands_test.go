package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRunList(t *testing.T) {
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
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/list-sockets" {
			t.Errorf("Expected /list-sockets path, got %s", r.URL.Path)
		}

		// Return a list of sockets
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{"socket1.sock", "socket2.sock"})
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up paths
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the command
	RunList(paths)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Check output
	if !strings.Contains(output, "socket1.sock") || !strings.Contains(output, "socket2.sock") {
		t.Errorf("Expected output to contain socket names, got: %s", output)
	}
}
