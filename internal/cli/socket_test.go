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
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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
		if r.URL.Path != "/socket/create" {
			t.Errorf("Expected /socket/create path, got %s", r.URL.Path)
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

		// Return a proper JSON response
		w.Header().Set("Content-Type", "application/json")
		response := management.Response[management.CreateResponse]{
			Status: "success",
			Response: management.CreateResponse{
				Socket: "/var/run/docker-proxy/test-socket.sock",
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("output", "text", "")
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
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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
		if r.URL.Path != "/socket/delete" {
			t.Errorf("Expected /socket/delete path, got %s", r.URL.Path)
		}
		if r.Header.Get("Socket-Path") != "/var/run/test-socket.sock" {
			t.Errorf("Expected Socket-Path header to be /var/run/test-socket.sock, got %s",
				r.Header.Get("Socket-Path"))
		}

		// Return a proper JSON response
		w.Header().Set("Content-Type", "application/json")
		response := management.Response[management.DeleteResponse]{
			Status: "success",
			Response: management.DeleteResponse{
				Message: "Socket deleted successfully",
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "text", "")
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
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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
		if r.URL.Path != "/socket/list" {
			t.Errorf("Expected /socket/list path, got %s", r.URL.Path)
		}

		// Return a proper JSON response
		w.Header().Set("Content-Type", "application/json")
		response := management.Response[management.ListResponse]{
			Status: "success",
			Response: management.ListResponse{
				Sockets: []string{"socket1.sock", "socket2.sock"},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up paths
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Set up test command
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "text", "")

	// Capture stdout
	output := captureOutput(func() {
		RunList(cmd, paths)
	})

	// Check output
	if !strings.Contains(output, "socket1.sock") || !strings.Contains(output, "socket2.sock") {
		t.Errorf("Expected output to contain socket names, got: %s", output)
	}
}

func TestRunDescribe(t *testing.T) {
	// Create a temporary directory for the test socket
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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
		if r.URL.Path != "/socket/describe" {
			t.Errorf("Expected /socket/describe path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("socket") != "test-socket.sock" {
			t.Errorf("Expected socket query param to be test-socket.sock, got %s",
				r.URL.Query().Get("socket"))
		}

		// Return a proper JSON response
		w.Header().Set("Content-Type", "application/json")
		response := management.Response[management.DescribeResponse]{
			Status: "success",
			Response: management.DescribeResponse{
				Config: map[string]interface{}{
					"rules": map[string]interface{}{
						"acls": []map[string]interface{}{
							{
								"match": map[string]interface{}{
									"path":   "/test",
									"method": "GET",
								},
								"action": "allow",
							},
						},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	server.Listener = l
	server.Start()
	defer server.Close()

	// Set up test command and arguments
	cmd := &cobra.Command{}
	cmd.Flags().String("output", "text", "")
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

func TestRunClean(t *testing.T) {
	// Create a temporary directory for the test socket
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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
		if r.URL.Path != "/socket/clean" {
			t.Errorf("Expected /socket/clean path, got %s", r.URL.Path)
		}

		_, err := w.Write([]byte("All sockets have been removed successfully"))
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))

	server.Listener = l
	server.Start()
	defer server.Close()

	cmd := &cobra.Command{}
	// Set up paths
	paths := &management.SocketPaths{
		Management: socketPath,
	}

	// Capture stdout
	output := captureOutput(func() {
		RunClean(cmd, paths)
	})

	// Check output
	if !strings.Contains(output, "All sockets have been removed successfully") {
		t.Errorf("Expected output to contain success message, got: %s", output)
	}
}
