package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"docker-socket-proxy/internal/application"
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/logging"
)

// errorResponse represents a JSON error response
type errorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// writeJSON writes a JSON response with the given status code
func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	if err := writeJSON(w, status, errorResponse{Success: false, Error: msg}); err != nil {
		logging.GetLogger().Error("failed to write JSON error response", "error", err)
	}
}

// ManagementHandler handles HTTP requests for socket management
type ManagementHandler struct {
	socketService application.SocketService
	logger        *slog.Logger
}

// NewManagementHandler creates a new management handler
func NewManagementHandler(socketService application.SocketService) *ManagementHandler {
	return &ManagementHandler{
		socketService: socketService,
		logger:        logging.GetLogger(),
	}
}

// CreateSocketHandler handles socket creation requests
func (h *ManagementHandler) CreateSocketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req CreateSocketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Convert request to domain config
	config := domain.SocketConfig{
		Name:            req.Name,
		ListenAddress:   req.ListenAddress,
		DockerDaemonURL: req.DockerDaemonURL,
		Rules:           convertRulesToDomain(req.Rules),
	}

	// Create socket using service
	socket, err := h.socketService.CreateSocket(r.Context(), config)
	if err != nil {
		h.logger.Error("Failed to create socket", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create socket")
		return
	}

	// Return success response
	response := CreateSocketResponse{
		Success: true,
		Socket:  socket,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// ListSocketsHandler handles socket listing requests
func (h *ManagementHandler) ListSocketsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// List sockets using service
	socketPaths, err := h.socketService.ListSockets(r.Context())
	if err != nil {
		h.logger.Error("Failed to list sockets", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list sockets")
		return
	}

	// Return success response
	response := ListSocketsResponse{
		Success: true,
		Sockets: socketPaths,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// DescribeSocketHandler handles socket description requests
func (h *ManagementHandler) DescribeSocketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get socket name from query parameter
	socketName := r.URL.Query().Get("socket")
	if socketName == "" {
		writeJSONError(w, http.StatusBadRequest, "Socket parameter is required")
		return
	}

	// Describe socket using service
	config, err := h.socketService.DescribeSocket(r.Context(), socketName)
	if err != nil {
		h.logger.Error("Failed to describe socket", "socket", socketName, "error", err)
		writeJSONError(w, http.StatusNotFound, "Socket not found")
		return
	}

	// Return success response
	response := DescribeSocketResponse{
		Success: true,
		Config:  config,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// DeleteSocketHandler handles socket deletion requests
func (h *ManagementHandler) DeleteSocketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get socket name from query parameter or header
	socketName := r.URL.Query().Get("socket")
	if socketName == "" {
		socketName = r.Header.Get("Socket-Path")
	}
	if socketName == "" {
		writeJSONError(w, http.StatusBadRequest, "Socket parameter is required")
		return
	}

	// Delete socket using service
	err := h.socketService.DeleteSocket(r.Context(), socketName)
	if err != nil {
		h.logger.Error("Failed to delete socket", "socket", socketName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete socket")
		return
	}

	// Return success response
	response := DeleteSocketResponse{
		Success: true,
		Message: "Socket deleted successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// CleanSocketsHandler handles socket cleanup requests
func (h *ManagementHandler) CleanSocketsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Clean sockets using service
	err := h.socketService.CleanSockets(r.Context())
	if err != nil {
		h.logger.Error("Failed to clean sockets", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to clean sockets")
		return
	}

	// Return success response
	response := CleanSocketsResponse{
		Success: true,
		Message: "All sockets cleaned successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// HealthHandler handles health check requests
func (h *ManagementHandler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := HealthResponse{
		Status:  "healthy",
		Message: "Docker Socket Proxy is running",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.GetLogger().Error("failed to encode response", "error", err)
	}
}

// convertRulesToDomain converts HTTP request rules to domain rules
func convertRulesToDomain(rules []RuleRequest) []domain.Rule {
	domainRules := make([]domain.Rule, len(rules))
	for i, rule := range rules {
		domainRules[i] = domain.Rule{
			Match: domain.Match{
				Path:     rule.Match.Path,
				Method:   rule.Match.Method,
				Contains: rule.Match.Contains,
			},
			Actions: convertActionsToDomain(rule.Actions),
		}
	}
	return domainRules
}

// convertActionsToDomain converts HTTP request actions to domain actions
func convertActionsToDomain(actions []ActionRequest) []domain.Action {
	domainActions := make([]domain.Action, len(actions))
	for i, action := range actions {
		actionType, err := domain.ParseActionType(action.Action)
		if err != nil {
			// Default to allow if parsing fails
			actionType = domain.ActionAllow
		}

		domainActions[i] = domain.Action{
			Type:     actionType,
			Reason:   action.Reason,
			Contains: action.Contains,
			Update:   action.Update,
		}
	}
	return domainActions
}

// HTTP Request/Response types

type CreateSocketRequest struct {
	Name            string        `json:"name"`
	ListenAddress   string        `json:"listen_address"`
	DockerDaemonURL string        `json:"docker_daemon_url"`
	Rules           []RuleRequest `json:"rules"`
}

type RuleRequest struct {
	Match   MatchRequest    `json:"match"`
	Actions []ActionRequest `json:"actions"`
}

type MatchRequest struct {
	Path     string         `json:"path"`
	Method   string         `json:"method"`
	Contains map[string]any `json:"contains,omitempty"`
}

type ActionRequest struct {
	Action   string         `json:"action"`
	Reason   string         `json:"reason,omitempty"`
	Contains map[string]any `json:"contains,omitempty"`
	Update   map[string]any `json:"update,omitempty"`
}

type CreateSocketResponse struct {
	Success bool          `json:"success"`
	Socket  domain.Socket `json:"socket"`
}

type ListSocketsResponse struct {
	Success bool     `json:"success"`
	Sockets []string `json:"sockets"`
}

type DescribeSocketResponse struct {
	Success bool                `json:"success"`
	Config  domain.SocketConfig `json:"config"`
}

type DeleteSocketResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type CleanSocketsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}
