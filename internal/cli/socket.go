package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"

	"github.com/spf13/cobra"
)

// RunCreate executes the socket create command
func RunCreate(cmd *cobra.Command, paths *management.SocketPaths) {
	configPath, _ := cmd.Flags().GetString("config")

	var socketConfig *config.SocketConfig
	if configPath != "" {
		var err error
		socketConfig, err = config.LoadSocketConfig(configPath)
		if err != nil {
			exitWithError("Error loading configuration: %v", err)
		}
	}

	// Encode the config as JSON
	var body io.Reader
	if socketConfig != nil {
		configJSON, err := json.Marshal(socketConfig)
		if err != nil {
			exitWithError("Error encoding configuration: %v", err)
		}
		body = bytes.NewReader(configJSON)
	}

	// Create the client
	client := createClient(paths.Management)

	// Send the request
	resp, err := client.Post("http://localhost/socket/create", "application/json", body)
	if err != nil {
		exitWithError("Failed to create socket: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	responseBody, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Server error: %v", err)
	}

	fmt.Println(string(responseBody))
}

// RunDelete executes the socket delete command
func RunDelete(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	if len(args) == 0 {
		exitWithError("Error: socket path is required")
	}

	socketPath := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the delete request
	req, err := http.NewRequest("DELETE", "http://localhost/socket/delete", nil)
	if err != nil {
		exitWithError("Error creating request: %v", err)
	}

	// Add the socket path as a query parameter
	q := req.URL.Query()
	q.Add("socket", socketPath)
	req.URL.RawQuery = q.Encode()

	// Add the Socket-Path header for backward compatibility
	req.Header.Set("Socket-Path", socketPath)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		exitWithError("Error sending request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	_, err = handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to delete socket: %v", err)
	}

	fmt.Printf("Socket %s deleted successfully\n", socketPath)
}

// RunDescribe executes the describe command
func RunDescribe(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	if len(args) == 0 {
		exitWithError("Error: socket name is required")
	}

	socketName := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the describe request
	req, err := http.NewRequest("GET", "http://localhost/socket/describe", nil)
	if err != nil {
		exitWithError("Error creating request: %v", err)
	}

	// Add the socket name as a query parameter
	q := req.URL.Query()
	q.Add("socket", socketName)
	req.URL.RawQuery = q.Encode()

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		exitWithError("Error sending request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	body, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to describe socket: %v", err)
	}

	// Print the YAML configuration
	fmt.Printf("Configuration for socket %s:\n\n%s\n", socketName, string(body))
}

// RunList executes the list command
func RunList(paths *management.SocketPaths) {
	// Create the client
	client := createClient(paths.Management)

	// Create the list request
	req, err := http.NewRequest("GET", "http://localhost/socket/list", nil)
	if err != nil {
		exitWithError("Error creating request: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		exitWithError("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Handle the response
	body, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to list sockets: %v", err)
	}

	// Parse the response
	var sockets []string
	if err := json.Unmarshal(body, &sockets); err != nil {
		exitWithError("Error parsing response: %v", err)
	}

	// Print the sockets
	if len(sockets) == 0 {
		fmt.Println("No sockets found")
		return
	}

	fmt.Println("Available sockets:")
	for _, socket := range sockets {
		fmt.Printf("  %s\n", socket)
	}
}

// RunClean executes the clean command
func RunClean(cmd *cobra.Command, paths *management.SocketPaths) {
	// Create the client
	client := createClient(paths.Management)

	// Create the clean request
	req, err := http.NewRequest("DELETE", "http://localhost/socket/clean", nil)
	if err != nil {
		exitWithError("Error creating request: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		exitWithError("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Handle the response
	_, err = handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to clean sockets: %v", err)
	}

	fmt.Println("All sockets have been removed successfully")
}
