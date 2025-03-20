package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"docker-socket-proxy/internal/logging"

	"gopkg.in/yaml.v3"
)

type SocketConfig struct {
	Config ConfigSet `json:"config" yaml:"config"`
	Rules  RuleSet   `json:"rules" yaml:"rules"`
}

type ConfigSet struct {
	PropagateSocket string `yaml:"propagate_socket"`
}

type RuleSet struct {
	ACLs     []Rule        `yaml:"acls"`
	Rewrites []RewriteRule `yaml:"rewrites"`
}

type RewriteRule struct {
	Match    Match     `yaml:"match"`
	Patterns []Pattern `yaml:"patterns"`
}

type Pattern struct {
	Field  string      `yaml:"field"`           // Supports dot notation for nested fields
	Match  interface{} `yaml:"match,omitempty"` // Optional match value for updates
	Value  interface{} `yaml:"value,omitempty"` // The value to set/update
	Action string      `yaml:"action"`          // "replace" (default), "upsert", or "delete"
}

type Rule struct {
	Match  Match  `yaml:"match"`
	Action string `yaml:"action"`
	Reason string `yaml:"reason,omitempty"`
}

type Match struct {
	Path     string                 `yaml:"path,omitempty" json:"path,omitempty"`
	Method   string                 `yaml:"method,omitempty" json:"method,omitempty"`
	Contains map[string]interface{} `yaml:"contains,omitempty" json:"contains,omitempty"`
}

// ValidateConfig validates the socket configuration
func ValidateConfig(config *SocketConfig) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Validate ACLs
	if len(config.Rules.ACLs) == 0 {
		return fmt.Errorf("at least one ACL rule must be defined")
	}

	// Validate ACL rules
	for i, rule := range config.Rules.ACLs {
		if err := validateACLRule(i, rule); err != nil {
			return err
		}
	}

	// Validate rewrite rules
	for i, rule := range config.Rules.Rewrites {
		if err := validateRewriteRule(i, rule); err != nil {
			return err
		}
	}

	return nil
}

func validateACLRule(i int, rule Rule) error {
	// Validate action
	if rule.Action != "allow" && rule.Action != "deny" {
		return fmt.Errorf("rule %d: action must be either 'allow' or 'deny', got '%s'", i, rule.Action)
	}

	// Validate match criteria
	if rule.Match.Path == "" && rule.Match.Method == "" && len(rule.Match.Contains) == 0 {
		return fmt.Errorf("rule %d: at least one match criteria (path, method, or contains) must be specified", i)
	}

	// Require reason for deny rules
	if rule.Action == "deny" && rule.Reason == "" {
		return fmt.Errorf("rule %d: deny rules must specify a reason", i)
	}

	return nil
}

func validateRewriteRule(i int, rule RewriteRule) error {
	// Validate match criteria
	if rule.Match.Path == "" && rule.Match.Method == "" && len(rule.Match.Contains) == 0 {
		return fmt.Errorf("rewrite rule %d: at least one match criteria (path, method, or contains) must be specified", i)
	}

	// Validate patterns
	if len(rule.Patterns) == 0 {
		return fmt.Errorf("rewrite rule %d: at least one pattern must be specified", i)
	}

	for j, pattern := range rule.Patterns {
		if pattern.Field == "" {
			return fmt.Errorf("rewrite rule %d, pattern %d: field must be specified", i, j)
		}

		if pattern.Action != "replace" && pattern.Action != "upsert" && pattern.Action != "delete" {
			return fmt.Errorf("rewrite rule %d, pattern %d: action must be 'replace', 'upsert', or 'delete', got '%s'", i, j, pattern.Action)
		}

		if pattern.Action == "replace" && pattern.Match == nil {
			return fmt.Errorf("rewrite rule %d, pattern %d: replace action requires a match value", i, j)
		}

		if pattern.Action != "delete" && pattern.Value == nil {
			return fmt.Errorf("rewrite rule %d, pattern %d: %s action requires a value", i, j, pattern.Action)
		}
	}

	return nil
}

func LoadSocketConfig(path string) (*SocketConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config SocketConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &config, nil
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

// MatchValue handles pattern matching for different types
func MatchValue(pattern interface{}, value interface{}) bool {
	switch p := pattern.(type) {
	case string:
		v, ok := value.(string)
		if !ok {
			return false
		}
		return MatchPattern(p, v)

	case []interface{}:
		v, ok := value.([]interface{})
		if !ok {
			return false
		}
		// For arrays, check if any pattern matches any value
		for _, pItem := range p {
			pStr, ok := pItem.(string)
			if !ok {
				continue
			}
			for _, vItem := range v {
				vStr, ok := vItem.(string)
				if !ok {
					continue
				}
				if MatchPattern(pStr, vStr) {
					return true
				}
			}
		}
		return false

	default:
		// For non-string/array types, use exact matching
		return reflect.DeepEqual(pattern, value)
	}
}

// GetPropagationRules returns rewrite rules for socket propagation if enabled
func (c *SocketConfig) GetPropagationRules() []RewriteRule {
	if c.Config.PropagateSocket == "" {
		return nil
	}

	log := logging.GetLogger()
	log.Debug("Creating propagation rules",
		"socket", c.Config.PropagateSocket)

	return []RewriteRule{
		{
			Match: Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Patterns: []Pattern{
				{
					Field:  "HostConfig.Binds",
					Action: "upsert",
					Value:  []interface{}{fmt.Sprintf("%s:%s:ro", c.Config.PropagateSocket, c.Config.PropagateSocket)},
				},
			},
		},
	}
}
