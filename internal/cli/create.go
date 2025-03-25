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
		exitWithError("Error requesting socket: %v", err)
	}
	defer resp.Body.Close()

	// Handle the response
	responseBody, err := handleResponse(resp, http.StatusOK)
	if err != nil {
		exitWithError("Server error: %v", err)
	}

	fmt.Println(string(responseBody))
}
