package cli

import (
	"fmt"
	"net/http"

	"docker-socket-proxy/internal/management"

	"github.com/spf13/cobra"
)

func RunDelete(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	if len(args) == 0 {
		exitWithError("Error: socket path is required")
	}

	socketPath := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the delete request
	req, err := http.NewRequest("DELETE", "http://localhost/delete-socket", nil)
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
	defer resp.Body.Close()

	// Handle the response
	_, err = handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to delete socket: %v", err)
	}

	fmt.Printf("Socket %s deleted successfully\n", socketPath)
}
