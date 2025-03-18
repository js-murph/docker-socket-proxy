package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"docker-socket-proxy/internal/management"

	"github.com/spf13/cobra"
)

func RunDescribe(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	if len(args) == 0 {
		fmt.Println("Error: socket name is required")
		os.Exit(1)
	}

	socketName := args[0]

	// Connect to the management socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", paths.Management)
			},
		},
	}

	// Create the describe request
	req, err := http.NewRequest("GET", "http://localhost/describe-socket", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		os.Exit(1)
	}

	// Add the socket name as a query parameter
	q := req.URL.Query()
	q.Add("socket", socketName)
	req.URL.RawQuery = q.Encode()

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Failed to describe socket: %s\n", body)
		os.Exit(1)
	}

	// Read and print the YAML configuration
	config, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(string(config))
}
