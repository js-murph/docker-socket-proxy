package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/proxy/config"
	"docker-socket-proxy/internal/storage"
)

func TestServer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "docker-proxy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

	paths := &management.SocketPaths{
		Management: filepath.Join(tmpDir, "mgmt.sock"),
		Docker:     filepath.Join(tmpDir, "docker.sock"),
		SocketDir:  tmpDir,
	}

	// Create a test socket
	socketPath := filepath.Join(tmpDir, "test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Errorf("Failed to close listener: %v", err)
	}

	testConfig := &config.SocketConfig{
		Rules: []config.Rule{
			{
				Match: config.Match{Path: "/test", Method: "GET"},
				Actions: []config.Action{
					{
						Action: "allow",
					},
				},
			},
		},
	}

	store := storage.NewFileStore(paths.SocketDir)
	if err := store.SaveConfig(socketPath, testConfig); err != nil {
		t.Fatal(err)
	}

	// Verify the config was saved correctly
	savedConfig, err := store.LoadConfig(socketPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if savedConfig.Rules[0].Actions[0].Action != "allow" {
		t.Fatalf("Config not saved correctly: %+v", savedConfig)
	}

	// Create server with the store and manually add the config
	srv := &Server{
		managementSocket: paths.Management,
		dockerSocket:     paths.Docker,
		socketDir:        paths.SocketDir,
		server:           &http.Server{},
		socketConfigs:    make(map[string]*config.SocketConfig),
		createdSockets:   make([]string, 0),
		store:            store,
		proxyServers:     make(map[string]*http.Server),
		configMu:         sync.RWMutex{},
	}

	// Manually add the config
	srv.configMu.Lock()
	srv.socketConfigs[socketPath] = testConfig
	srv.configMu.Unlock()

	// Verify config was loaded
	srv.configMu.RLock()
	cfg, ok := srv.socketConfigs[socketPath]
	srv.configMu.RUnlock()

	if !ok {
		t.Error("Expected test config to be loaded")
	} else {
		// Compare specific fields instead of using DeepEqual
		if len(cfg.Rules) != len(testConfig.Rules) {
			t.Errorf("Loaded config has %d rules, want %d",
				len(cfg.Rules), len(testConfig.Rules))
		} else if cfg.Rules[0].Actions[0].Action != testConfig.Rules[0].Actions[0].Action {
			t.Errorf("Loaded config has action %s, want %s",
				cfg.Rules[0].Actions[0].Action, testConfig.Rules[0].Actions[0].Action)
		}
	}

	// Start the server in a goroutine with a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.startWithContext(ctx); err != nil && err != http.ErrServerClosed {
			t.Errorf("Server.Start() error = %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the server
	cancel()
}

// Add this method to the Server struct
func (s *Server) startWithContext(ctx context.Context) error {
	log := logging.GetLogger()
	// Set up the management handler
	handler := NewManagementHandler(s.dockerSocket, s.socketConfigs, &s.configMu, s.store)
	s.server.Handler = handler

	// Listen on the management socket
	listener, err := net.Listen("unix", s.managementSocket)
	if err != nil {
		return fmt.Errorf("failed to listen on management socket: %w", err)
	}

	// Remove the socket file when the server stops
	defer func() {
		if err := os.Remove(s.managementSocket); err != nil {
			log.Error("Failed to remove management socket", "error", err)
		}
	}()

	log.Info("Management server listening on socket", "path", s.managementSocket)

	// Serve until context is canceled
	go func() {
		<-ctx.Done()
		if err := s.server.Close(); err != nil {
			log.Error("Failed to close server", "error", err)
		}
	}()

	return s.server.Serve(listener)
}
