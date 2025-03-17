package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"
)

type SocketConfig struct {
	Rules RuleSet `yaml:"rules"`
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
	Path     string                 `yaml:"path,omitempty"`
	Method   string                 `yaml:"method,omitempty"`
	Contains map[string]interface{} `yaml:"contains,omitempty"`
}

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

	// Validate method if specified
	if rule.Match.Method != "" {
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true,
			"DELETE": true, "PATCH": true, "HEAD": true,
		}
		if !validMethods[rule.Match.Method] {
			return fmt.Errorf("rule %d: invalid HTTP method '%s'", i, rule.Match.Method)
		}
	}

	// Validate path format if specified
	if rule.Match.Path != "" && rule.Match.Path[0] != '/' {
		return fmt.Errorf("rule %d: path must start with '/', got '%s'", i, rule.Match.Path)
	}

	return nil
}

func validateRewriteRule(i int, rule RewriteRule) error {
	// Validate match criteria
	if rule.Match.Path == "" && rule.Match.Method == "" && len(rule.Match.Contains) == 0 {
		return fmt.Errorf("rewrite rule %d: at least one match criteria (path, method, or contains) must be specified", i)
	}

	// Validate method if specified
	if rule.Match.Method != "" {
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true,
			"DELETE": true, "PATCH": true, "HEAD": true,
		}
		if !validMethods[rule.Match.Method] {
			return fmt.Errorf("rewrite rule %d: invalid HTTP method '%s'", i, rule.Match.Method)
		}
	}

	// Validate path format if specified
	if rule.Match.Path != "" && rule.Match.Path[0] != '/' {
		return fmt.Errorf("rewrite rule %d: path must start with '/', got '%s'", i, rule.Match.Path)
	}

	// Validate patterns
	if len(rule.Patterns) == 0 {
		return fmt.Errorf("rewrite rule %d: at least one pattern must be specified", i)
	}

	for j, pattern := range rule.Patterns {
		if pattern.Field == "" {
			return fmt.Errorf("rewrite rule %d, pattern %d: field cannot be empty", i, j)
		}

		if pattern.Action == "" {
			pattern.Action = "replace" // Default action
		}

		validActions := map[string]bool{
			"replace": true,
			"upsert":  true,
			"delete":  true,
		}
		if !validActions[pattern.Action] {
			return fmt.Errorf("rewrite rule %d, pattern %d: invalid action '%s'", i, j, pattern.Action)
		}

		if pattern.Action != "delete" {
			if pattern.Value == nil {
				return fmt.Errorf("rewrite rule %d, pattern %d: value must be specified for non-delete actions", i, j)
			}

			// Match is only required for replace action
			if pattern.Action == "replace" && pattern.Match == nil {
				return fmt.Errorf("rewrite rule %d, pattern %d: match value must be specified for replace action", i, j)
			}

			// Validate type compatibility
			if pattern.Match != nil && !validateTypeCompatibility(pattern.Match, pattern.Value) {
				return fmt.Errorf("rewrite rule %d, pattern %d: match and value must be of compatible types", i, j)
			}
		} else if pattern.Match != nil && !validateMatchType(pattern.Match) {
			return fmt.Errorf("rewrite rule %d, pattern %d: invalid match type for delete action", i, j)
		}
	}

	return nil
}

func validateTypeCompatibility(match, value interface{}) bool {
	switch match.(type) {
	case string:
		_, ok := value.(string)
		return ok
	case bool:
		_, ok := value.(bool)
		return ok
	case float64:
		_, ok := value.(float64)
		return ok
	case []interface{}:
		_, ok := value.([]interface{})
		return ok
	}
	return reflect.TypeOf(match) == reflect.TypeOf(value)
}

func validateMatchType(match interface{}) bool {
	switch match.(type) {
	case string, bool:
		return true
	default:
		return false
	}
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
