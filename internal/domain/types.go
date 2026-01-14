package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Socket represents a proxy endpoint with associated rules
type Socket struct {
	Path   string
	Config SocketConfig
}

// SocketConfig represents the complete configuration for a socket
type SocketConfig struct {
	Name            string    `json:"name" yaml:"name"`
	ListenAddress   string    `json:"listen_address" yaml:"listen_address"`
	DockerDaemonURL string    `json:"docker_daemon_url" yaml:"docker_daemon_url"`
	Config          ConfigSet `json:"config" yaml:"config"`
	Rules           []Rule    `json:"rules" yaml:"rules"`
}

// ConfigSet represents general socket settings
type ConfigSet struct {
	PropagateSocket string `json:"propagate_socket" yaml:"propagate_socket"`
}

// Rule represents a rule that matches requests and defines actions
type Rule struct {
	Match   Match    `json:"match" yaml:"match"`
	Actions []Action `json:"actions" yaml:"actions"`
}

// Match represents criteria for when a rule applies
type Match struct {
	Path     string         `json:"path" yaml:"path"`
	Method   string         `json:"method" yaml:"method"`
	Contains map[string]any `json:"contains,omitempty" yaml:"contains,omitempty"`
}

// Action represents what to do when a rule matches
type Action struct {
	Type     ActionType     `json:"type" yaml:"type"`
	Reason   string         `json:"reason,omitempty" yaml:"reason,omitempty"`
	Contains map[string]any `json:"contains,omitempty" yaml:"contains,omitempty"`
	Update   map[string]any `json:"update,omitempty" yaml:"update,omitempty"`
}

// Request represents an HTTP request with parsed body
type Request struct {
	Method string
	Path   string
	Body   map[string]any
}

// NewRequest creates a new Request from an HTTP request
func NewRequest(r *http.Request) Request {
	body := make(map[string]any)

	// Parse request body if it exists and is JSON
	if r.Body != nil && r.Header.Get("Content-Type") == "application/json" {
		// Read the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			// Parse JSON
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				// If JSON parsing fails, leave body empty
				body = make(map[string]any)
			}
		}
		// Restore the body for potential re-reading
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   body,
	}
}

// ActionType represents the type of action to take
type ActionType string

const (
	ActionAllow   ActionType = "allow"
	ActionDeny    ActionType = "deny"
	ActionUpsert  ActionType = "upsert"
	ActionReplace ActionType = "replace"
	ActionDelete  ActionType = "delete"
)

// ToActionType converts a string to ActionType
func ToActionType(s string) ActionType {
	switch s {
	case "allow":
		return ActionAllow
	case "deny":
		return ActionDeny
	case "upsert":
		return ActionUpsert
	case "replace":
		return ActionReplace
	case "delete":
		return ActionDelete
	default:
		return ActionAllow // Default to allow for backward compatibility
	}
}

// ParseActionType parses a string to ActionType, returning an error if invalid
func ParseActionType(s string) (ActionType, error) {
	switch s {
	case "allow":
		return ActionAllow, nil
	case "deny":
		return ActionDeny, nil
	case "upsert":
		return ActionUpsert, nil
	case "replace":
		return ActionReplace, nil
	case "delete":
		return ActionDelete, nil
	default:
		return "", fmt.Errorf("invalid action type: %s", s)
	}
}

// String returns the string representation of ActionType
func (at ActionType) String() string {
	return string(at)
}

// EvaluationResult represents the result of rule evaluation
type EvaluationResult struct {
	Allowed      bool
	Reason       string
	Modified     bool
	ModifiedBody map[string]any
}

// NewEvaluationResult creates a new EvaluationResult
func NewEvaluationResult(allowed bool, reason string) EvaluationResult {
	return EvaluationResult{
		Allowed:      allowed,
		Reason:       reason,
		Modified:     false,
		ModifiedBody: nil,
	}
}

// NewModifiedEvaluationResult creates a new EvaluationResult with modifications
func NewModifiedEvaluationResult(allowed bool, reason string, modifiedBody map[string]any) EvaluationResult {
	return EvaluationResult{
		Allowed:      allowed,
		Reason:       reason,
		Modified:     true,
		ModifiedBody: modifiedBody,
	}
}

// ContextKey represents a key for context values
type ContextKey string

const (
	SocketContextKey  ContextKey = "socket"
	RequestContextKey ContextKey = "request"
)

// LoadSocketConfig loads a socket configuration from a file
func LoadSocketConfig(configPath string) (*SocketConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config SocketConfig

	// Determine if the file is YAML or JSON based on extension
	if strings.HasSuffix(configPath, ".yaml") || strings.HasSuffix(configPath, ".yml") {
		// Parse YAML
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config file: %w", err)
		}
	} else if strings.HasSuffix(configPath, ".json") {
		// Assume it's JSON
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported config file extension: %s", filepath.Ext(configPath))
	}

	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &config, nil
}

// ValidateConfig validates the socket configuration
func ValidateConfig(config *SocketConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate rules
	if len(config.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	// Validate each rule
	for i, rule := range config.Rules {
		if err := validateRule(i, rule); err != nil {
			return err
		}
	}

	return nil
}

// validateRule validates a rule
func validateRule(index int, rule Rule) error {
	// Validate match
	if rule.Match.Path == "" {
		return fmt.Errorf("rule %d: path is required", index)
	}

	// Validate actions
	if len(rule.Actions) == 0 {
		return fmt.Errorf("rule %d: at least one action is required", index)
	}

	// Validate each action
	for i, action := range rule.Actions {
		if err := validateAction(index, i, action); err != nil {
			return err
		}
	}

	return nil
}

// validateAction validates an action
func validateAction(ruleIndex, actionIndex int, action Action) error {
	// Handle empty action type (backward compatibility)
	if action.Type == "" {
		action.Type = ActionAllow // Default to allow for backward compatibility
	}

	// Validate action type
	switch action.Type {
	case ActionAllow:
		// Allow actions are always valid
	case ActionDeny:
		// Deny actions require a reason
		if action.Reason == "" {
			return fmt.Errorf("rule %d, action %d: deny action requires a reason", ruleIndex, actionIndex)
		}
	case ActionUpsert, ActionReplace, ActionDelete:
		// Rewrite actions require contains and/or update fields
		if action.Type != ActionDelete && len(action.Update) == 0 {
			return fmt.Errorf("rule %d, action %d: %s action requires update field",
				ruleIndex, actionIndex, action.Type)
		}
		if len(action.Contains) == 0 && action.Type != ActionUpsert {
			return fmt.Errorf("rule %d, action %d: %s action requires contains field",
				ruleIndex, actionIndex, action.Type)
		}
	default:
		return fmt.Errorf("rule %d, action %d: invalid action: %s", ruleIndex, actionIndex, action.Type)
	}

	return nil
}
