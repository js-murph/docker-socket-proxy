package action

import (
	"docker-socket-proxy/internal/domain"
)

// ActionExecutor executes actions on requests
type ActionExecutor struct{}

// NewActionExecutor creates a new ActionExecutor
func NewActionExecutor() *ActionExecutor {
	return &ActionExecutor{}
}

// ExecuteAction executes a single action
func (e *ActionExecutor) ExecuteAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	actionType := action.Type

	switch actionType {
	case domain.ActionAllow:
		return e.executeAllowAction(action, req)
	case domain.ActionDeny:
		return e.executeDenyAction(action, req)
	case domain.ActionUpsert:
		return e.executeUpsertAction(action, req)
	case domain.ActionReplace:
		return e.executeReplaceAction(action, req)
	case domain.ActionDelete:
		return e.executeDeleteAction(action, req)
	default:
		// Unknown action, allow by default
		return domain.NewEvaluationResult(true, "")
	}
}

// executeAllowAction executes an allow action
func (e *ActionExecutor) executeAllowAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	// TODO: Check if action's contains criteria match when body matcher is implemented
	// For now, we'll just return allow

	return domain.NewEvaluationResult(true, action.Reason)
}

// executeDenyAction executes a deny action
func (e *ActionExecutor) executeDenyAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	// TODO: Check if action's contains criteria match when body matcher is implemented
	// For now, we'll just return deny

	return domain.NewEvaluationResult(false, action.Reason)
}

// executeUpsertAction executes an upsert action
func (e *ActionExecutor) executeUpsertAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	// This would use a modifier in a real implementation
	// For now, we'll just return allow
	return domain.NewEvaluationResult(true, "")
}

// executeReplaceAction executes a replace action
func (e *ActionExecutor) executeReplaceAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	// This would use a modifier in a real implementation
	// For now, we'll just return allow
	return domain.NewEvaluationResult(true, "")
}

// executeDeleteAction executes a delete action
func (e *ActionExecutor) executeDeleteAction(action domain.Action, req domain.Request) domain.EvaluationResult {
	// This would use a modifier in a real implementation
	// For now, we'll just return allow
	return domain.NewEvaluationResult(true, "")
}
