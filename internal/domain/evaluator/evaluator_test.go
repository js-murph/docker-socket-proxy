package evaluator

import (
	"docker-socket-proxy/internal/domain"
	"testing"
)

func TestUnit_RuleEvaluator_GivenNoRules_WhenEvaluating_ThenAllows(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "GET",
		Path:   "/v1.42/containers/json",
		Body:   map[string]interface{}{},
	}

	result := evaluator.EvaluateRules(req, []domain.Rule{})

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
}

func TestUnit_RuleEvaluator_GivenAllowRule_WhenEvaluating_ThenAllows(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "GET",
		Path:   "/v1.42/containers/json",
		Body:   map[string]interface{}{},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/.*",
				Method: "GET",
			},
			Actions: []domain.Action{
				{Type: domain.ActionAllow, Reason: "Allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if result.Reason != "Allowed" {
		t.Errorf("RuleEvaluator.EvaluateRules() reason = %v, want 'Allowed'", result.Reason)
	}
}

func TestUnit_RuleEvaluator_GivenDenyRule_WhenEvaluating_ThenDenies(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body:   map[string]interface{}{"Privileged": true},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Privileged": true,
				},
			},
			Actions: []domain.Action{
				{Type: domain.ActionDeny, Reason: "Privileged containers not allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want false", result.Allowed)
	}
	if result.Reason != "Privileged containers not allowed" {
		t.Errorf("RuleEvaluator.EvaluateRules() reason = %v, want 'Privileged containers not allowed'", result.Reason)
	}
}

func TestUnit_RuleEvaluator_GivenNonMatchingRule_WhenEvaluating_ThenAllows(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "GET",
		Path:   "/v1.42/containers/json",
		Body:   map[string]interface{}{},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{Type: domain.ActionDeny, Reason: "Not allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
}

func TestUnit_RuleEvaluator_GivenMultipleRules_WhenEvaluating_ThenFirstMatchWins(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body:   map[string]interface{}{"Env": []interface{}{"DEBUG=true"}},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
				Contains: map[string]interface{}{
					"Env": []interface{}{"DEBUG=true"},
				},
			},
			Actions: []domain.Action{
				{Type: domain.ActionAllow, Reason: "Debug mode allowed"},
			},
		},
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{Type: domain.ActionDeny, Reason: "Not allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if result.Reason != "Debug mode allowed" {
		t.Errorf("RuleEvaluator.EvaluateRules() reason = %v, want 'Debug mode allowed'", result.Reason)
	}
}

func TestUnit_RuleEvaluator_GivenUpsertAction_WhenEvaluating_ThenModifiesBody(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body:   map[string]interface{}{"Image": "nginx"},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{
					Type: domain.ActionUpsert,
					Update: map[string]interface{}{
						"Env": []interface{}{"DEBUG=true"},
					},
				},
				{Type: domain.ActionAllow, Reason: "Modified and allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if !result.Modified {
		t.Errorf("RuleEvaluator.EvaluateRules() modified = %v, want true", result.Modified)
	}
	if result.ModifiedBody == nil {
		t.Errorf("RuleEvaluator.EvaluateRules() modifiedBody = nil, want non-nil")
	}
}

func TestUnit_RuleEvaluator_GivenReplaceAction_WhenEvaluating_ThenModifiesBody(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body:   map[string]interface{}{"Image": "nginx"},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{
					Type: domain.ActionReplace,
					Update: map[string]interface{}{
						"Image": "alpine",
					},
				},
				{Type: domain.ActionAllow, Reason: "Modified and allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if !result.Modified {
		t.Errorf("RuleEvaluator.EvaluateRules() modified = %v, want true", result.Modified)
	}
}

func TestUnit_RuleEvaluator_GivenDeleteAction_WhenEvaluating_ThenModifiesBody(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body: map[string]interface{}{
			"Image": "nginx",
			"Env":   []interface{}{"DEBUG=true"},
		},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{
					Type: domain.ActionDelete,
					Contains: map[string]interface{}{
						"Env": []interface{}{"DEBUG=true"},
					},
				},
				{Type: domain.ActionAllow, Reason: "Modified and allowed"},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if !result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want true", result.Allowed)
	}
	if !result.Modified {
		t.Errorf("RuleEvaluator.EvaluateRules() modified = %v, want true", result.Modified)
	}
}

func TestUnit_RuleEvaluator_GivenActionWithContains_WhenEvaluating_ThenChecksContains(t *testing.T) {
	evaluator := NewRuleEvaluator()
	req := domain.Request{
		Method: "POST",
		Path:   "/v1.42/containers/create",
		Body: map[string]interface{}{
			"Env": []interface{}{"DEBUG=true"},
		},
	}

	rules := []domain.Rule{
		{
			Match: domain.Match{
				Path:   "/v1.*/containers/create",
				Method: "POST",
			},
			Actions: []domain.Action{
				{
					Type:   domain.ActionDeny,
					Reason: "Debug mode not allowed",
					Contains: map[string]interface{}{
						"Env": []interface{}{"DEBUG=true"},
					},
				},
			},
		},
	}

	result := evaluator.EvaluateRules(req, rules)

	if result.Allowed {
		t.Errorf("RuleEvaluator.EvaluateRules() allowed = %v, want false", result.Allowed)
	}
	if result.Reason != "Debug mode not allowed" {
		t.Errorf("RuleEvaluator.EvaluateRules() reason = %v, want 'Debug mode not allowed'", result.Reason)
	}
}
