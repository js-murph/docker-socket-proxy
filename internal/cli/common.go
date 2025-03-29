package cli

import (
	"context"
	"docker-socket-proxy/internal/cli/output"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// For testing - allows us to override os.Exit
var osExit = os.Exit

// createClient creates an HTTP client that connects to the management socket
func createClient(managementSocket string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", managementSocket)
			},
		},
	}
}

// handleResponse handles common response processing and error handling
func handleResponse(resp *http.Response, expectedStatus int) ([]byte, error) {
	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return body, nil
}

// exitWithError prints an error message and exits with code 1
func exitWithError(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
	osExit(1)
}

// getOutput returns an Output instance based on the output format flag
func getOutput(cmd *cobra.Command) *output.Output {
	format, _ := cmd.Flags().GetString("output")
	return output.New(format, os.Stdout)
}

// getErrorOutput returns an Output instance for error messages
func getErrorOutput(cmd *cobra.Command) *output.Output {
	format, _ := cmd.Flags().GetString("output")
	return output.New(format, os.Stderr)
}
