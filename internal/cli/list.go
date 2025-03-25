package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"docker-socket-proxy/internal/management"
)

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
