package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/server"
)

func TestProxyIntegration(t *testing.T) {
	// Skip if not running integration tests
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test")
	}

	// Create temporary directory for test sockets
	tmpDir, err := os.MkdirTemp("", "proxy-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test paths
	paths := &management.SocketPaths{
		Management: filepath.Join(tmpDir, "mgmt.sock"),
		Docker:     filepath.Join(tmpDir, "docker.sock"),
	}

	// Start the server
	srv := server.New(paths)
	go func() {
		if err := srv.Start(); err != nil {
			t.Errorf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test creating a proxy socket
	t.Run("create and use proxy socket", func(t *testing.T) {
		config := map[string]interface{}{
			"rules": map[string]interface{}{
				"acls": []interface{}{
					map[string]interface{}{
						"match": map[string]interface{}{
							"path":   "/v1.*/containers/json",
							"method": "GET",
						},
						"action": "allow",
					},
				},
			},
		}

		// Create proxy socket
		proxySocket, err := createProxySocket(paths.Management, config)
		if err != nil {
			t.Fatalf("Failed to create proxy socket: %v", err)
		}

		// Test making a request through the proxy
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", proxySocket)
				},
			},
		}

		resp, err := client.Get("http://unix/v1.42/containers/json")
		if err != nil {
			t.Fatalf("Failed to make request through proxy: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", resp.Status)
		}
	})
}

func createProxySocket(managementSocket string, config map[string]interface{}) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", managementSocket)
			},
		},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	resp, err := client.Post("http://unix/create-socket", "application/json",
		bytes.NewReader(configJSON))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	socketPath, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(socketPath), nil
}
