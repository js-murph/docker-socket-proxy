package cli

import (
	"fmt"
	"net/http"

	"docker-socket-proxy/internal/management"

	"github.com/spf13/cobra"
)

func RunDescribe(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	if len(args) == 0 {
		exitWithError("Error: socket name is required")
	}

	socketName := args[0]

	// Create the client
	client := createClient(paths.Management)

	// Create the describe request
	req, err := http.NewRequest("GET", "http://localhost/describe-socket", nil)
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
	defer resp.Body.Close()

	// Handle the response
	body, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Failed to describe socket: %v", err)
	}

	// Print the YAML configuration
	fmt.Printf("Configuration for socket %s:\n\n%s\n", socketName, string(body))
}
