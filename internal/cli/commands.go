package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

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
			fmt.Printf("Error loading configuration: %v\n", err)
			os.Exit(1)
		}
	}

	// Encode the config as JSON
	var body io.Reader
	if socketConfig != nil {
		configJSON, err := json.Marshal(socketConfig)
		if err != nil {
			fmt.Printf("Error encoding configuration: %v\n", err)
			os.Exit(1)
		}
		body = bytes.NewReader(configJSON)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", paths.Management)
			},
		},
	}

	resp, err := client.Post("http://unix/create-socket", "application/json", body)
	if err != nil {
		fmt.Printf("Error requesting socket: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Server error: %s\n", body)
		os.Exit(1)
	}

	socketPath, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(socketPath))
}

func RunDelete(cmd *cobra.Command, args []string, paths *management.SocketPaths) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", paths.Management)
			},
		},
	}

	req, err := http.NewRequest("DELETE", "http://unix/delete-socket", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Socket-Path", args[0])

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error deleting socket: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Server error: %s\n", body)
		os.Exit(1)
	}
}
