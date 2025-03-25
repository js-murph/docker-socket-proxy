package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"docker-socket-proxy/internal/cli"
	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/management"
	"docker-socket-proxy/internal/server"

	"github.com/spf13/cobra"
)

func main() {
	paths := management.NewSocketPaths()
	var srv *server.Server

	var rootCmd = &cobra.Command{
		Use:   "docker-socket-proxy",
		Short: "A proxy for Docker socket management",
	}

	var daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Run the proxy server daemon",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			srv, err = server.NewServer(paths.Management, paths.Docker, paths.SocketDir)
			if err != nil {
				slog.Error("Failed to create server", "error", err)
				os.Exit(1)
			}
			runDaemon(srv)
		},
	}

	daemonCmd.Flags().StringVar(&paths.Management, "management-socket",
		management.DefaultManagementSocketPath, "Path to the management socket")
	daemonCmd.Flags().StringVar(&paths.Docker, "docker-socket",
		management.DefaultDockerSocketPath, "Path to the Docker daemon socket")

	var socketCmd = &cobra.Command{
		Use:   "socket",
		Short: "Manage Docker proxy sockets",
	}

	var createCmd = &cobra.Command{
		Use:   "create",
		Short: "Create a new Docker proxy socket",
		Run: func(cmd *cobra.Command, args []string) {
			cli.RunCreate(cmd, paths)
		},
	}

	// Add config file flag to create command
	createCmd.Flags().StringP("config", "c", "", "Path to socket configuration file (yaml)")

	var deleteCmd = &cobra.Command{
		Use:   "delete [socket-path]",
		Short: "Delete a Docker proxy socket",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cli.RunDelete(cmd, args, paths)
		},
	}

	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List available proxy sockets",
		Run: func(cmd *cobra.Command, args []string) {
			cli.RunList(paths)
		},
	}

	var describeCmd = &cobra.Command{
		Use:   "describe [socket-name]",
		Short: "Show configuration for a Docker proxy socket",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cli.RunDescribe(cmd, args, paths)
		},
	}

	var cleanCmd = &cobra.Command{
		Use:   "clean",
		Short: "Remove all proxy sockets",
		Run: func(cmd *cobra.Command, args []string) {
			cli.RunClean(cmd, paths)
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

func runDaemon(srv *server.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	slog.Info("Shutting down server...")
	srv.Stop()
}
