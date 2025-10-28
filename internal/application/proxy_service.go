package application

import (
	"bytes"
	"context"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/domain/evaluator"
	"docker-socket-proxy/internal/logging"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ProxyService defines the interface for proxy operations
type ProxyService interface {
	ProcessRequest(ctx context.Context, req *http.Request, socketName string) (*http.Response, error)
	ProcessRequestWithConfig(ctx context.Context, req *http.Request, config domain.SocketConfig) (*http.Response, error)
	EvaluateRules(ctx context.Context, req domain.Request, config domain.SocketConfig) (domain.EvaluationResult, error)
}

// DockerClient defines the interface for communicating with Docker daemon
type DockerClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// proxyService implements ProxyService
type proxyService struct {
	dockerClient  DockerClient
	evaluator     *evaluator.RuleEvaluator
	socketManager SocketManager
}

// NewProxyService creates a new ProxyService
func NewProxyService(dockerClient DockerClient, socketManager SocketManager) ProxyService {
	return &proxyService{
		dockerClient:  dockerClient,
		evaluator:     evaluator.NewRuleEvaluator(),
		socketManager: socketManager,
	}
}

// ProcessRequest processes a request through the proxy
func (p *proxyService) ProcessRequest(ctx context.Context, req *http.Request, socketName string) (*http.Response, error) {
	logging.GetLogger().Debug("Processing request", "socketName", socketName)

	// Load socket configuration by name
	config, exists := p.socketManager.GetSocket(socketName)
	if !exists {
		logging.GetLogger().Debug("Socket not found, using empty configuration", "socketName", socketName)
		// If socket not found, use empty configuration (allow all)
		config = domain.SocketConfig{
			Rules: []domain.Rule{},
		}
	} else {
		logging.GetLogger().Debug("Socket found", "socketName", socketName, "rules", len(config.Rules))
	}

	// Use ProcessRequestWithConfig with the loaded configuration
	return p.ProcessRequestWithConfig(ctx, req, config)
}

// ProcessRequestWithConfig processes a request through the proxy with explicit configuration
func (p *proxyService) ProcessRequestWithConfig(ctx context.Context, req *http.Request, config domain.SocketConfig) (*http.Response, error) {
	// Convert HTTP request to domain request
	domainReq := domain.NewRequest(req)

	// Evaluate rules
	result, err := p.EvaluateRules(ctx, domainReq, config)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate rules: %w", err)
	}

	// Check if request is allowed
	if !result.Allowed {
		// Return 403 Forbidden
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Body:       http.NoBody,
		}, nil
	}

	// Apply modifications if any
	if result.Modified && result.ModifiedBody != nil {
		// Modify the request body with the modified content
		modifiedBody, err := json.Marshal(result.ModifiedBody)
		if err != nil {
			logging.GetLogger().Error("Failed to marshal modified body", "error", err, "modifiedBody", result.ModifiedBody)
			return nil, fmt.Errorf("failed to marshal modified body: %w", err)
		}

		// Update the request body
		req.Body = io.NopCloser(bytes.NewReader(modifiedBody))
		req.ContentLength = int64(len(modifiedBody))

		logging.GetLogger().Debug("Request body modified", "socket", config.Name)
	}

	// Forward request to Docker daemon
	resp, err := p.dockerClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to forward request to Docker daemon: %w", err)
	}

	return resp, nil
}

// EvaluateRules evaluates rules against a request
func (p *proxyService) EvaluateRules(ctx context.Context, req domain.Request, config domain.SocketConfig) (domain.EvaluationResult, error) {
	// Use the rule evaluator to evaluate rules
	result := p.evaluator.EvaluateRules(req, config.Rules)

	// Log the evaluation result
	logging.GetLogger().Debug("Rule evaluation result",
		"allowed", result.Allowed,
		"reason", result.Reason,
		"modified", result.Modified,
		"path", req.Path,
		"method", req.Method,
	)

	return result, nil
}

// httpDockerClient implements DockerClient using HTTP
type httpDockerClient struct {
	client *http.Client
	url    string
}

// NewHTTPDockerClient creates a new HTTP Docker client
func NewHTTPDockerClient(url string) DockerClient {
	return &httpDockerClient{
		client: &http.Client{},
		url:    url,
	}
}

// Do forwards the request to the Docker daemon
func (c *httpDockerClient) Do(req *http.Request) (*http.Response, error) {
	// Update the request URL to point to Docker daemon
	req.URL.Scheme = "http"
	req.URL.Host = c.url

	// Forward the request
	return c.client.Do(req)
}

// unixDockerClient implements DockerClient using Unix socket
type unixDockerClient struct {
	client *http.Client
	socket string
}

// NewUnixDockerClient creates a new Unix socket Docker client
func NewUnixDockerClient(socket string) DockerClient {
	return &unixDockerClient{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", socket)
				},
			},
		},
		socket: socket,
	}
}

// Do forwards the request to the Docker daemon via Unix socket
func (c *unixDockerClient) Do(req *http.Request) (*http.Response, error) {
	// Update the request URL to point to Docker daemon
	req.URL.Scheme = "http"
	req.URL.Host = "docker"

	// Forward the request
	return c.client.Do(req)
}
