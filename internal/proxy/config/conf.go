package config

import (
	"docker-socket-proxy/internal/logging"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SocketConfig represents the socket configuration
type SocketConfig struct {
	Config ConfigSet `json:"config" yaml:"config"`
	Rules  []Rule    `json:"rules" yaml:"rules"`
}

type ConfigSet struct {
	PropagateSocket string `json:"propagate_socket" yaml:"propagate_socket"`
}

// Rule represents a rule in the new format
type Rule struct {
	Match   Match    `json:"match" yaml:"match"`
	Actions []Action `json:"actions" yaml:"actions"`
}

// Match represents a match criteria
type Match struct {
	Path     string         `json:"path" yaml:"path"`
	Method   string         `json:"method" yaml:"method"`
	Contains map[string]any `json:"contains,omitempty" yaml:"contains,omitempty"`
}

// Action represents an action to take
type Action struct {
	Action   string         `json:"action" yaml:"action"`
	Reason   string         `json:"reason,omitempty" yaml:"reason,omitempty"`
	Contains map[string]any `json:"contains,omitempty" yaml:"contains,omitempty"`
	Update   map[string]any `json:"update,omitempty" yaml:"update,omitempty"`
}

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
	// Validate action type
	switch action.Action {
	case "allow":
		// Allow actions are always valid
	case "deny":
		// Deny actions require a reason
		if action.Reason == "" {
			return fmt.Errorf("rule %d, action %d: deny action requires a reason", ruleIndex, actionIndex)
		}
	case "upsert", "replace", "delete":
		// Rewrite actions require contains and/or update fields
		if action.Action != "delete" && len(action.Update) == 0 {
			return fmt.Errorf("rule %d, action %d: %s action requires update field",
				ruleIndex, actionIndex, action.Action)
		}
		if len(action.Contains) == 0 && action.Action != "upsert" {
			return fmt.Errorf("rule %d, action %d: %s action requires contains field",
				ruleIndex, actionIndex, action.Action)
		}
	default:
		return fmt.Errorf("rule %d, action %d: invalid action: %s", ruleIndex, actionIndex, action.Action)
	}

	return nil
}

// GetPropagationRules returns rules for socket propagation if enabled
func (c *SocketConfig) GetPropagationRules() []Rule {
	if c.Config.PropagateSocket == "" {
		return nil
	}

	log := logging.GetLogger()
	log.Debug("Creating propagation rules",
		"socket", c.Config.PropagateSocket)

	return []Rule{
		{
			Match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []Action{
				{
					Action: "upsert",
					Update: map[string]any{
						"HostConfig": map[string]any{
							"Binds": []interface{}{
								fmt.Sprintf("%s:%s:ro", c.Config.PropagateSocket, c.Config.PropagateSocket),
							},
						},
					},
				},
			},
		},
	}
}
