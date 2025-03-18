package cli

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// captureOutput captures stdout during a function execution
func captureOutput(f func()) string {
	// Save and restore stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the function
	f()

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout

	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String()
}

// mockResponseBody creates a mock http.Response with the given status code and body
func mockResponseBody(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestHandleResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedStatus int
		wantErr        bool
	}{
		{
			name:           "successful response",
			statusCode:     http.StatusOK,
			responseBody:   "success",
			expectedStatus: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "error response",
			statusCode:     http.StatusBadRequest,
			responseBody:   "error message",
			expectedStatus: http.StatusOK,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := mockResponseBody(tt.statusCode, tt.responseBody)

			body, err := handleResponse(resp, tt.expectedStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && string(body) != tt.responseBody {
				t.Errorf("handleResponse() body = %v, want %v", string(body), tt.responseBody)
			}
		})
	}
}

func TestExitWithError(t *testing.T) {
	// Save the original os.Exit function
	origExit := osExit
	defer func() { osExit = origExit }()

	var exitCode int
	osExit = func(code int) {
		exitCode = code
		// Don't actually exit in tests
	}

	output := captureOutput(func() {
		exitWithError("Test error: %s", "message")
	})

	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(output, "Test error: message") {
		t.Errorf("Expected output to contain error message, got: %s", output)
	}
}
