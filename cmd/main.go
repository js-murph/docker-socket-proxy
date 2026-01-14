package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"docker-socket-proxy/internal/application"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/infrastructure/repository"
	httpInterface "docker-socket-proxy/internal/interfaces/http"
	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"

	"github.com/spf13/cobra"
)

func main() {
	// Initialize logging
	logger := logging.GetLogger()

	// Create dependency container
	container := NewDependencyContainer()

	var rootCmd = &cobra.Command{
		Use:   "docker-socket-proxy",
		Short: "A proxy for Docker socket management",
	}

	rootCmd.PersistentFlags().String("output", "yaml", "Output format (text|json|yaml|silent)")

	var daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Run the proxy server daemon",
		Run: func(cmd *cobra.Command, args []string) {
			runDaemon(container, logger)
		},
	}

	daemonCmd.Flags().StringVar(&container.Config.ManagementSocket, "management-socket",
		management.DefaultManagementSocketPath, "Path to the management socket")
	daemonCmd.Flags().StringVar(&container.Config.DockerSocket, "docker-socket",
		management.DefaultDockerSocketPath, "Path to the Docker daemon socket")
	daemonCmd.Flags().StringVar(&container.Config.SocketDir, "socket-dir",
		management.DefaultSocketDir, "Directory for socket files")
	daemonCmd.Flags().BoolVar(&container.Config.UseFileStorage, "file-storage",
		true, "Use file-based storage for socket configurations")

	var socketCmd = &cobra.Command{
		Use:   "socket",
		Short: "Manage Docker proxy sockets",
	}

	var createCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new Docker proxy socket",
		Run: func(cmd *cobra.Command, args []string) {
			runCreate(container, cmd, args)
		},
	}

	// Add config file flag to create command
	createCmd.Flags().StringP("config", "c", "", "Path to socket configuration file (yaml)")

	var deleteCmd = &cobra.Command{
		Use:   "delete [socket-path]",
		Short: "Delete a Docker proxy socket",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDelete(container, cmd, args)
		},
	}

	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List available proxy sockets",
		Run: func(cmd *cobra.Command, args []string) {
			runList(container, cmd, args)
		},
	}

	var describeCmd = &cobra.Command{
		Use:   "describe [socket-name]",
		Short: "Show configuration for a Docker proxy socket",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runDescribe(container, cmd, args)
		},
	}

	var cleanCmd = &cobra.Command{
		Use:   "clean",
		Short: "Remove all proxy sockets",
		Run: func(cmd *cobra.Command, args []string) {
			runClean(container, cmd, args)
		},
	}

	socketCmd.AddCommand(createCmd, deleteCmd, listCmd, describeCmd, cleanCmd)
	rootCmd.AddCommand(daemonCmd, socketCmd)

	var logLevel string
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (debug, info, warn, error)")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		switch strings.ToLower(logLevel) {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		logging.SetLevel(level)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// DependencyContainer holds all dependencies
type DependencyContainer struct {
	Config        Config
	Repository    application.SocketRepository
	SocketManager application.SocketManager
	SocketService application.SocketService
	ProxyService  application.ProxyService
	Server        *httpInterface.Server
}

// Config holds application configuration
type Config struct {
	ManagementSocket string
	DockerSocket     string
	SocketDir        string
	UseFileStorage   bool
}

// NewDependencyContainer creates a new dependency container
func NewDependencyContainer() *DependencyContainer {
	return &DependencyContainer{
		Config: Config{
			ManagementSocket: management.DefaultManagementSocketPath,
			DockerSocket:     management.DefaultDockerSocketPath,
			SocketDir:        management.DefaultSocketDir,
			UseFileStorage:   true,
		},
	}
}

// InitializeServices initializes all services with proper dependencies
func (c *DependencyContainer) InitializeServices() error {
	// Create repository
	if c.Config.UseFileStorage {
		c.Repository = repository.NewFileSocketRepository(c.Config.SocketDir)
	} else {
		c.Repository = repository.NewInMemorySocketRepository()
	}

	// Create socket manager
	c.SocketManager = application.NewSocketManager(c.Config.SocketDir)

	// Create services
	c.SocketService = application.NewSocketService(c.Repository, c.SocketManager)
	c.ProxyService = application.NewProxyService(&MockDockerClient{}, c.SocketManager)

	// Create server
	c.Server = httpInterface.NewServer(c.SocketService, c.ProxyService, c.Config.ManagementSocket)

	return nil
}

// runDaemon runs the proxy server daemon
func runDaemon(container *DependencyContainer, logger *slog.Logger) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		logger.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	// Start server
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := container.Server.Start(); err != nil {
			logger.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	logger.Info("Shutting down server...")
	if err := container.Server.Stop(context.TODO()); err != nil {
		logger.Error("Error stopping server", "error", err)
	}
}

// runCreate creates a new socket
func runCreate(container *DependencyContainer, cmd *cobra.Command, args []string) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		slog.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	// Get config file path
	configFile, _ := cmd.Flags().GetString("config")
	if configFile == "" {
		slog.Error("Config file is required")
		os.Exit(1)
	}

	// Load configuration from file
	config, err := loadSocketConfig(configFile)
	if err != nil {
		slog.Error("Failed to load socket configuration", "error", err)
		os.Exit(1)
	}

	// Create socket
	socket, err := container.SocketService.CreateSocket(cmd.Context(), config)
	if err != nil {
		slog.Error("Failed to create socket", "error", err)
		os.Exit(1)
	}

	slog.Info("Socket created successfully", "socket", socket.Path)
}

// runDelete deletes a socket
func runDelete(container *DependencyContainer, cmd *cobra.Command, args []string) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		slog.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	socketPath := args[0]

	// Delete socket
	err := container.SocketService.DeleteSocket(cmd.Context(), socketPath)
	if err != nil {
		slog.Error("Failed to delete socket", "error", err)
		os.Exit(1)
	}

	slog.Info("Socket deleted successfully", "socket", socketPath)
}

// runList lists all sockets
func runList(container *DependencyContainer, cmd *cobra.Command, args []string) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		slog.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	// List sockets
	sockets, err := container.SocketService.ListSockets(cmd.Context())
	if err != nil {
		slog.Error("Failed to list sockets", "error", err)
		os.Exit(1)
	}

	// Output sockets
	for _, socket := range sockets {
		slog.Info("Socket", "path", socket)
	}
}

// runDescribe describes a socket
func runDescribe(container *DependencyContainer, cmd *cobra.Command, args []string) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		slog.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	socketName := args[0]

	// Describe socket
	config, err := container.SocketService.DescribeSocket(cmd.Context(), socketName)
	if err != nil {
		slog.Error("Failed to describe socket", "error", err)
		os.Exit(1)
	}

	// Output socket configuration
	slog.Info("Socket configuration", "name", config.Name, "listen_address", config.ListenAddress)
}

// runClean cleans all sockets
func runClean(container *DependencyContainer, cmd *cobra.Command, args []string) {
	// Initialize services
	if err := container.InitializeServices(); err != nil {
		slog.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	// Clean sockets
	err := container.SocketService.CleanSockets(cmd.Context())
	if err != nil {
		slog.Error("Failed to clean sockets", "error", err)
		os.Exit(1)
	}

	slog.Info("All sockets cleaned successfully")
}

// loadSocketConfig loads socket configuration from file
func loadSocketConfig(filename string) (domain.SocketConfig, error) {
	// TODO: Implement file loading
	return domain.SocketConfig{}, nil
}

// MockDockerClient is a mock implementation of DockerClient
type MockDockerClient struct{}

func (m *MockDockerClient) Do(req *http.Request) (*http.Response, error) {
	// TODO: Implement actual Docker client
	return &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}
