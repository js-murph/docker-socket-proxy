package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"docker-socket-proxy/internal/management"
)

func RunList(paths *management.SocketPaths) {
	// Connect to the management socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", paths.Management)
			},
		},
	}

	// Create the list request
	req, err := http.NewRequest("GET", "http://localhost/list-sockets", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		os.Exit(1)
	}

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
		fmt.Printf("Failed to list sockets: %s\n", body)
		os.Exit(1)
	}

	// Parse the response
	var sockets []string
	if err := json.NewDecoder(resp.Body).Decode(&sockets); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		os.Exit(1)
	}

	// Print the sockets
	if len(sockets) == 0 {
		fmt.Println("No sockets found")
		return
	}

	for _, socket := range sockets {
		fmt.Printf("  %s\n", socket)
	}
}
