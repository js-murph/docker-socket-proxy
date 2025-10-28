package evaluator

import (
	"docker-socket-proxy/internal/domain"
	"docker-socket-proxy/internal/domain/matcher"
	"docker-socket-proxy/internal/domain/modifier"
)

// RuleEvaluator evaluates rules against requests
type RuleEvaluator struct{}

// NewRuleEvaluator creates a new RuleEvaluator
func NewRuleEvaluator() *RuleEvaluator {
	return &RuleEvaluator{}
}

// EvaluateRules evaluates all rules against a request and returns the result
func (e *RuleEvaluator) EvaluateRules(req domain.Request, rules []domain.Rule) domain.EvaluationResult {
	// If no rules, allow by default
	if len(rules) == 0 {
		return domain.NewEvaluationResult(true, "")
	}

	// Process each rule in order
	for _, rule := range rules {
		// Check if rule matches the request
		matches := e.ruleMatches(req, rule.Match)
		if matches {
			// Rule matches, evaluate its actions
			result := e.evaluateActions(req, rule.Actions)
			if result.Allowed || !result.Allowed {
				// First matching rule wins
				return result
			}
		}
	}

	// If no rules match, allow by default
	return domain.NewEvaluationResult(true, "")
}

// ruleMatches checks if a request matches a rule's match criteria
func (e *RuleEvaluator) ruleMatches(req domain.Request, match domain.Match) bool {
	// Create composite matcher for the request
	matchers := []matcher.RequestMatcher{}

	// Add path matcher if specified
	if match.Path != "" {
		matchers = append(matchers, matcher.NewPathMatcher(match.Path))
	}

	// Add method matcher if specified
	if match.Method != "" {
		matchers = append(matchers, matcher.NewMethodMatcher(match.Method))
	}

	// Add body matcher if specified
	if len(match.Contains) > 0 {
		matchers = append(matchers, matcher.NewBodyMatcher(match.Contains))
	}

	// If no matchers, rule matches everything
	if len(matchers) == 0 {
		return true
	}

	// Create composite matcher and evaluate
	compositeMatcher := matcher.NewCompositeRequestMatcher(matchers...)
	return compositeMatcher.MatchesRequest(req)
}

// evaluateActions evaluates the actions for a matching rule
func (e *RuleEvaluator) evaluateActions(req domain.Request, actions []domain.Action) domain.EvaluationResult {
	modified := false
	modifiedBody := make(map[string]interface{})

	// Copy original body for modification
	if req.Body != nil {
		for k, v := range req.Body {
			modifiedBody[k] = v
		}
	}

	// Process each action in order
	for _, action := range actions {
		actionType := action.Type

		switch actionType {
		case domain.ActionDeny:
			// Check if action's contains criteria match
			if len(action.Contains) > 0 && req.Body != nil {
				bodyMatcher := matcher.NewBodyMatcher(action.Contains)
				if !bodyMatcher.MatchesRequest(req) {
					continue
				}
			}
			return domain.NewEvaluationResult(false, action.Reason)

		case domain.ActionAllow:
			// Check if action's contains criteria match
			if len(action.Contains) > 0 && req.Body != nil {
				bodyMatcher := matcher.NewBodyMatcher(action.Contains)
				if !bodyMatcher.MatchesRequest(req) {
					continue
				}
			}
			// Apply any modifications before allowing
			if modified {
				return domain.NewModifiedEvaluationResult(true, action.Reason, modifiedBody)
			}
			return domain.NewEvaluationResult(true, action.Reason)

		case domain.ActionUpsert:
			if req.Body != nil && len(action.Update) > 0 {
				upsertModifier := modifier.NewUpsertModifier(action.Update)
				newBody, modResult := upsertModifier.Modify(modifiedBody)
				if modResult {
					modified = true
					modifiedBody = newBody
				}
			}

		case domain.ActionReplace:
			if req.Body != nil && len(action.Update) > 0 {
				// Check if action's contains criteria match
				if len(action.Contains) > 0 {
					bodyMatcher := matcher.NewBodyMatcher(action.Contains)
					if !bodyMatcher.MatchesRequest(req) {
						continue
					}
				}
				replaceModifier := modifier.NewReplaceModifier(action.Update)
				newBody, modResult := replaceModifier.Modify(modifiedBody)
				if modResult {
					modified = true
					modifiedBody = newBody
				}
			}

		case domain.ActionDelete:
			if req.Body != nil && len(action.Contains) > 0 {
				deleteModifier := modifier.NewDeleteModifier(action.Contains)
				newBody, modResult := deleteModifier.Modify(modifiedBody)
				if modResult {
					modified = true
					modifiedBody = newBody
				}
			}
		}
	}

	// If we get here, no explicit allow/deny was found
	// Return allow with any modifications
	if modified {
		return domain.NewModifiedEvaluationResult(true, "", modifiedBody)
	}
	return domain.NewEvaluationResult(true, "")
}
