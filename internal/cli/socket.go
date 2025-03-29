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
	"gopkg.in/yaml.v3"
)

// RunCreate executes the socket create command
func RunCreate(cmd *cobra.Command, paths *management.SocketPaths) {
	out := getOutput(cmd)
	errOut := getErrorOutput(cmd)

	configPath, _ := cmd.Flags().GetString("config")

	var socketConfig *config.SocketConfig
	if configPath != "" {
		var err error
		socketConfig, err = config.LoadSocketConfig(configPath)
		if err != nil {
			errOut.Error(fmt.Errorf("Error loading configuration: %v", err))
			osExit(1)
		}
	}

	// Encode the config as JSON
	var body io.Reader
	if socketConfig != nil {
		configJSON, err := json.Marshal(socketConfig)
		if err != nil {
			errOut.Error(fmt.Errorf("Error encoding configuration: %v", err))
			osExit(1)
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
		errOut.Error(fmt.Errorf("Server error: %v", err))
		osExit(1)
	}

	// Parse the JSON response
	var response management.Response[management.CreateResponse]
	if err := json.Unmarshal(responseBody, &response); err != nil {
		errOut.Error(fmt.Errorf("Failed to parse response: %v", err))
		osExit(1)
	}

	// Print in requested format
	if format, _ := cmd.Flags().GetString("output"); format == "text" {
		out.Print(response.Response.Socket)
	} else {
		out.Print(response.Response)
	}
}

// RunDelete executes the socket delete command
func RunDelete(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	out := getOutput(cmd)
	errOut := getErrorOutput(cmd)

	if len(args) == 0 {
		errOut.Error(fmt.Errorf("Error: socket path is required"))
		osExit(1)
	}

	socketPath := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the delete request
	req, err := http.NewRequest("DELETE", "http://localhost/socket/delete", nil)
	if err != nil {
		errOut.Error(fmt.Errorf("Error creating request: %v", err))
		osExit(1)
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
		errOut.Error(fmt.Errorf("Error sending request: %v", err))
		osExit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	responseBody, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		errOut.Error(fmt.Errorf("Failed to delete socket: %v", err))
		osExit(1)
	}

	// Parse the JSON response
	var response management.Response[management.DeleteResponse]
	if err := json.Unmarshal(responseBody, &response); err != nil {
		errOut.Error(fmt.Errorf("Failed to parse response: %v", err))
		osExit(1)
	}

	// Print in requested format
	if format, _ := cmd.Flags().GetString("output"); format == "text" {
		out.Print(response.Response.Message)
	} else {
		out.Print(response.Response)
	}
}

// RunDescribe executes the describe command
func RunDescribe(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	out := getOutput(cmd)
	errOut := getErrorOutput(cmd)

	if len(args) == 0 {
		errOut.Error(fmt.Errorf("Error: socket name is required"))
		osExit(1)
	}

	socketName := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the describe request
	req, err := http.NewRequest("GET", "http://localhost/socket/describe", nil)
	if err != nil {
		errOut.Error(fmt.Errorf("Error creating request: %v", err))
		osExit(1)
	}

	// Add the socket name as a query parameter
	q := req.URL.Query()
	q.Add("socket", socketName)
	req.URL.RawQuery = q.Encode()

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		errOut.Error(fmt.Errorf("Error sending request: %v", err))
		osExit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	responseBody, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		errOut.Error(fmt.Errorf("Failed to describe socket: %v", err))
		osExit(1)
	}

	// Parse the JSON response
	var response management.Response[management.DescribeResponse]
	if err := json.Unmarshal(responseBody, &response); err != nil {
		errOut.Error(fmt.Errorf("Error parsing response: %v", err))
		osExit(1)
	}

	// Print in requested format
	if format, _ := cmd.Flags().GetString("output"); format == "text" {
		if err := yaml.NewEncoder(out.Writer()).Encode(response.Response.Config); err != nil {
			errOut.Error(fmt.Errorf("Failed to encode config: %v", err))
			osExit(1)
		}
	} else {
		out.Print(response.Response)
	}
}

// RunList executes the list command
func RunList(cmd *cobra.Command, paths *management.SocketPaths) {
	out := getOutput(cmd)
	errOut := getErrorOutput(cmd)

	// Create the client
	client := createClient(paths.Management)

	// Create the list request
	req, err := http.NewRequest("GET", "http://localhost/socket/list", nil)
	if err != nil {
		errOut.Error(fmt.Errorf("Error creating request: %v", err))
		osExit(1)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		errOut.Error(fmt.Errorf("Error sending request: %v", err))
		osExit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	responseBody, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		errOut.Error(fmt.Errorf("Failed to list sockets: %v", err))
		osExit(1)
	}

	// Parse the response
	var response management.Response[management.ListResponse]
	if err := json.Unmarshal(responseBody, &response); err != nil {
		errOut.Error(fmt.Errorf("Error parsing response: %v", err))
		osExit(1)
	}

	// Print in requested format
	if format, _ := cmd.Flags().GetString("output"); format == "text" {
		for _, socket := range response.Response.Sockets {
			out.Print(socket)
		}
	} else {
		out.Print(response.Response)
	}
}

// RunClean executes the clean command
func RunClean(cmd *cobra.Command, paths *management.SocketPaths) {
	out := getOutput(cmd)
	errOut := getErrorOutput(cmd)

	// Create the client
	client := createClient(paths.Management)

	// Create the clean request
	req, err := http.NewRequest("DELETE", "http://localhost/socket/clean", nil)
	if err != nil {
		errOut.Error(fmt.Errorf("Error creating request: %v", err))
		osExit(1)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		errOut.Error(fmt.Errorf("Error sending request: %v", err))
		osExit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			exitWithError("Failed to close response body: %v", err)
		}
	}()

	// Handle the response
	_, err = handleResponse(resp, http.StatusOK)
	if err != nil {
		errOut.Error(fmt.Errorf("Failed to clean sockets: %v", err))
		osExit(1)
	}

	out.Success("All sockets have been removed successfully")
}
