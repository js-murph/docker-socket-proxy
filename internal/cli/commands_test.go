package cli

import (
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
	"docker-socket-proxy/internal/proxy/config"

	"github.com/spf13/cobra"
)

func TestRunCreate(t *testing.T) {
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
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/create-socket" {
			t.Errorf("Expected /create-socket path, got %s", r.URL.Path)
		}

		// Check if there's a config in the request
		if r.Body != nil && r.Header.Get("Content-Type") == "application/json" {
			var cfg config.SocketConfig
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("Failed to read request body: %v", err)
			}

			// Only try to decode if there's content
			if len(body) > 0 {
				if err := json.Unmarshal(body, &cfg); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("/var/run/docker-proxy/test-socket.sock"))
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Capture stdout
	output := captureOutput(func() {
		RunCreate(cmd, paths)
	})

	// Check output
	if !strings.Contains(output, "/var/run/docker-proxy/test-socket.sock") {
		t.Errorf("Expected output to contain socket path, got: %s", output)
	}
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
		w.Write([]byte("Socket /var/run/test-socket.sock deleted successfully"))
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

	// Capture stdout
	output := captureOutput(func() {
		RunDelete(cmd, args, paths)
	})

	// Check output
	if !strings.Contains(output, "deleted successfully") {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}
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
	output := captureOutput(func() {
		RunList(paths)
	})

	// Check output
	if !strings.Contains(output, "socket1.sock") || !strings.Contains(output, "socket2.sock") {
		t.Errorf("Expected output to contain socket names, got: %s", output)
	}
}

func TestRunDescribe(t *testing.T) {
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
		if r.URL.Path != "/describe-socket" {
			t.Errorf("Expected /describe-socket path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("socket") != "test-socket.sock" {
			t.Errorf("Expected socket query param to be test-socket.sock, got %s",
				r.URL.Query().Get("socket"))
		}

		// Return a YAML config
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`rules:
  acls:
  - match:
      path: /test
      method: GET
    action: allow`))
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	args := []string{"test-socket.sock"}
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Capture stdout
	output := captureOutput(func() {
		RunDescribe(cmd, args, paths)
	})

	// Check output
	if !strings.Contains(output, "rules:") ||
		!strings.Contains(output, "acls:") ||
		!strings.Contains(output, "action: allow") {
		t.Errorf("Expected output to contain YAML config, got: %s", output)
	}
}
