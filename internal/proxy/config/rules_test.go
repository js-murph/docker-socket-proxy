package config

import (
	"os"
	"reflect"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *SocketConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &SocketConfig{
				Rules: RuleSet{
					ACLs: []Rule{
						{
							Match: Match{
								Path:   "/v1.*/containers",
								Method: "GET",
							},
							Action: "allow",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "no ACL rules",
			config: &SocketConfig{
				Rules: RuleSet{},
			},
			wantErr: true,
		},
		{
			name: "invalid ACL action",
			config: &SocketConfig{
				Rules: RuleSet{
					ACLs: []Rule{
						{
							Match: Match{
								Path: "/test",
							},
							Action: "invalid",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "test",
			value:   "test",
			want:    true,
		},
		{
			name:    "wildcard match",
			pattern: "test*",
			value:   "testing",
			want:    true,
		},
		{
			name:    "no match",
			pattern: "test",
			value:   "other",
			want:    false,
		},
		{
			name:    "invalid pattern",
			pattern: "[",
			value:   "test",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchPattern(tt.pattern, tt.value); got != tt.want {
				t.Errorf("MatchPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchValue(t *testing.T) {
	tests := []struct {
		name    string
		pattern interface{}
		value   interface{}
		want    bool
	}{
		{
			name:    "string match",
			pattern: "test*",
			value:   "testing",
			want:    true,
		},
		{
			name:    "boolean match",
			pattern: true,
			value:   true,
			want:    true,
		},
		{
			name:    "array match",
			pattern: []interface{}{"test*"},
			value:   []interface{}{"testing"},
			want:    true,
		},
		{
			name:    "type mismatch",
			pattern: "test",
			value:   123,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchValue(tt.pattern, tt.value); got != tt.want {
				t.Errorf("MatchValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadSocketConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid config",
			yaml: `
rules:
  acls:
    - match:
        path: "/v1.*/containers"
        method: "GET"
      action: "allow"
`,
			wantErr: false,
		},
		{
			name: "invalid yaml",
			yaml: `
invalid:
  - yaml
    format
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", "config-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			// Write test YAML
			if _, err := tmpfile.Write([]byte(tt.yaml)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			// Test loading config
			_, err = LoadSocketConfig(tmpfile.Name())
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadSocketConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetPropagationRules(t *testing.T) {
	config := &SocketConfig{
		Config: ConfigSet{
			PropagateSocket: "/var/run/docker.sock",
		},
	}

	rules := config.GetPropagationRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	rule := rules[0]
	if rule.Match.Path != "/v1.*/containers/create" || rule.Match.Method != "POST" {
		t.Error("unexpected rule match criteria")
	}

	if len(rule.Patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(rule.Patterns))
	}

	pattern := rule.Patterns[0]
	if pattern.Field != "HostConfig.Binds" || pattern.Action != "upsert" {
		t.Error("unexpected pattern configuration")
	}

	expectedValue := []interface{}{"/var/run/docker.sock:/var/run/docker.sock:ro"}
	if !reflect.DeepEqual(pattern.Value, expectedValue) {
		t.Errorf("expected value %v, got %v", expectedValue, pattern.Value)
	}
}

func TestValidateACLRuleWithRegex(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{
			name: "valid rule with regex path",
			rule: Rule{
				Match: Match{
					Path: "/v1\\.[0-9]+/containers/.*",
				},
				Action: "allow",
			},
			wantErr: false,
		},
		{
			name: "valid rule with regex method",
			rule: Rule{
				Match: Match{
					Path:   "/test",
					Method: "GET|POST",
				},
				Action: "allow",
			},
			wantErr: false,
		},
		{
			name: "valid rule with complex regex",
			rule: Rule{
				Match: Match{
					Path:   "/v1\\.[0-9]+/(containers|networks)/.*",
					Method: "^(GET|POST|PUT)$",
				},
				Action: "allow",
			},
			wantErr: false,
		},
		{
			name: "deny rule without reason",
			rule: Rule{
				Match: Match{
					Path: "/test/.*",
				},
				Action: "deny",
			},
			wantErr: true,
		},
		{
			name: "invalid action",
			rule: Rule{
				Match: Match{
					Path: "/test/.*",
				},
				Action: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateACLRule(0, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateACLRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
