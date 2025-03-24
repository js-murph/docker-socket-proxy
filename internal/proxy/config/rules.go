package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"docker-socket-proxy/internal/logging"
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
	Path     string                 `json:"path" yaml:"path"`
	Method   string                 `json:"method" yaml:"method"`
	Contains map[string]interface{} `json:"contains,omitempty" yaml:"contains,omitempty"`
}

// Action represents an action to take
type Action struct {
	Action   string                 `json:"action" yaml:"action"`
	Reason   string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	Contains map[string]interface{} `json:"contains,omitempty" yaml:"contains,omitempty"`
	Update   map[string]interface{} `json:"update,omitempty" yaml:"update,omitempty"`
}

// LoadSocketConfig loads a socket configuration from a file
func LoadSocketConfig(configPath string) (*SocketConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config SocketConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON config file: %w", err)
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

// MatchValue checks if a value matches an expected value
func MatchValue(expected, actual interface{}) bool {
	// Handle nil values
	if expected == nil && actual == nil {
		return true
	}
	if expected == nil || actual == nil {
		return false
	}

	// Handle different types
	switch exp := expected.(type) {
	case string:
		// String matching
		switch act := actual.(type) {
		case string:
			// If it looks like a regex, use regex matching
			if isRegexPattern(exp) {
				return matchRegex(exp, act)
			}
			// Otherwise, use exact matching
			return exp == act
		case []interface{}:
			// Check if any array element matches the string
			for _, item := range act {
				if str, ok := item.(string); ok {
					// If it looks like a regex, use regex matching
					if isRegexPattern(exp) {
						if matchRegex(exp, str) {
							return true
						}
					} else {
						// Otherwise, use exact matching
						if exp == str {
							return true
						}
					}
				}
			}
			return false
		default:
			return false
		}
	case []interface{}:
		// Array matching
		switch act := actual.(type) {
		case []interface{}:
			// Check if ALL items in expected are in actual
			for _, expItem := range exp {
				found := false
				expStr, isExpStr := expItem.(string)

				for _, actItem := range act {
					actStr, isActStr := actItem.(string)

					if isExpStr && isActStr {
						// If it looks like a regex, use regex matching
						if isRegexPattern(expStr) {
							if matchRegex(expStr, actStr) {
								found = true
								break
							}
						} else {
							// Otherwise, use exact matching
							if expStr == actStr {
								found = true
								break
							}
						}
					} else if reflect.DeepEqual(expItem, actItem) {
						found = true
						break
					}
				}

				if !found {
					return false // If any expected item is not found, return false
				}
			}
			return true // All expected items were found
		default:
			return false
		}
	case map[string]interface{}:
		// Map matching
		actMap, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}

		// Check if ALL keys in expected are in actual with matching values
		for key, expValue := range exp {
			actValue, exists := actMap[key]
			if !exists || !MatchValue(expValue, actValue) {
				return false
			}
		}
		return true
	default:
		// For other types, use direct equality
		return reflect.DeepEqual(expected, actual)
	}
}

// Helper function to check if a string is a regex pattern
func isRegexPattern(s string) bool {
	// These characters are definitely regex metacharacters when not escaped
	metaChars := []string{".", "*", "+", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|"}

	// Simple check: if it contains any metacharacters, treat it as a regex
	for _, char := range metaChars {
		if strings.Contains(s, char) {
			return true
		}
	}

	return false
}

// Helper function to match a regex pattern against a string
func matchRegex(pattern, s string) bool {
	// Try to compile and use the pattern as a regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		// If compilation fails, fall back to exact match
		return pattern == s
	}
	return re.MatchString(s)
}

// MatchPattern uses filepath.Match for wildcard pattern matching
func MatchPattern(pattern, value string) bool {
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		// Log error and fail closed
		return false
	}
	return matched
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
					Update: map[string]interface{}{
						"HostConfig": map[string]interface{}{
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
