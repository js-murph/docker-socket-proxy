package cli

import (
	"fmt"
	"net/http"

	"docker-socket-proxy/internal/management"
)

// RunClean executes the clean command
func RunClean(paths *management.SocketPaths) {
	// Create the client
	client := createClient(paths.Management)

	// Create the clean request
	req, err := http.NewRequest("DELETE", "http://localhost/sockets", nil)
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
