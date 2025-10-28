# Domain Language

This document defines the terminology used in the Docker Socket Proxy codebase. Use these terms consistently to maintain clarity and avoid confusion.

## Core Concepts

### Socket
A proxy endpoint that forwards Docker API requests with applied rules. Each socket has:
- A unique path (e.g., `/var/run/docker-proxy-abc123.sock`)
- A configuration with rules
- Associated permissions

### Rule
A combination of matching criteria and actions that determine how requests are handled. A rule consists of:
- **Match**: Criteria that determine when the rule applies
- **Actions**: What to do when the rule matches

### Matcher
A component that determines if a request matches specific criteria. Types include:
- **PathMatcher**: Matches request paths (exact or regex)
- **MethodMatcher**: Matches HTTP methods
- **BodyMatcher**: Matches request body content
- **ValueMatcher**: Matches individual values (strings, arrays, objects)

### Action
What to do when a rule matches. Types include:
- **AllowAction**: Allow the request to proceed
- **DenyAction**: Block the request with a reason
- **ModifyAction**: Transform the request before forwarding

### Modifier
A component that transforms request content. Types include:
- **UpsertModifier**: Add or update fields
- **ReplaceModifier**: Replace existing fields
- **DeleteModifier**: Remove fields

## Configuration Terms

### SocketConfig
The complete configuration for a socket, containing:
- **Config**: General settings (e.g., propagate_socket)
- **Rules**: Array of rules to apply

### Match
Criteria for when a rule applies:
- **Path**: Request path pattern (regex supported)
- **Method**: HTTP method pattern (regex supported)
- **Contains**: Body content that must be present

### Action
What to do when a rule matches:
- **Action**: Type of action (allow, deny, upsert, replace, delete)
- **Reason**: Human-readable reason (for deny actions)
- **Contains**: Additional criteria for the action
- **Update**: Fields to modify (for modify actions)

## Service Terms

### SocketService
Application service that manages socket lifecycle:
- Create sockets
- Delete sockets
- List sockets
- Describe socket configurations

### ProxyService
Application service that handles request proxying:
- Evaluate rules
- Apply modifications
- Forward requests to Docker daemon

### Repository
Data access layer for socket configurations:
- Save configurations
- Load configurations
- Delete configurations
- List configurations

## Technical Terms

### Rule Engine
The system that evaluates rules against requests:
- **RuleEvaluator**: Orchestrates rule evaluation
- **ActionExecutor**: Executes actions when rules match

### Request Processing
The flow of handling incoming requests:
1. **Parse**: Extract request details
2. **Match**: Check against rules
3. **Evaluate**: Determine actions to take
4. **Execute**: Apply actions (allow/deny/modify)
5. **Forward**: Send to Docker daemon (if allowed)

### Configuration Persistence
How socket configurations are stored:
- **FileRepository**: Stores configs as JSON files
- **InMemoryRepository**: Stores configs in memory (for testing)

## Anti-Patterns to Avoid

### Unclear Names
- ❌ `MatchValue` - too generic
- ✅ `StringValueMatcher` - specific and clear

### Mixed Responsibilities
- ❌ `ManagementHandler` doing socket creation AND HTTP handling
- ✅ Separate `SocketService` and `ManagementHandler`

### Context Abuse
- ❌ Using context for dependency injection
- ✅ Constructor injection for dependencies

### God Objects
- ❌ Large types with multiple responsibilities
- ✅ Small, focused types with single responsibility

## Examples

### Good Domain Language
```go
// Clear, specific naming
type SocketService interface {
    CreateSocket(ctx context.Context, config SocketConfig) (Socket, error)
    DeleteSocket(ctx context.Context, socketPath string) error
}

type RuleEvaluator interface {
    EvaluateRules(ctx context.Context, request Request, rules []Rule) ([]Action, error)
}

type ValueMatcher interface {
    Matches(value interface{}) bool
}
```

### Poor Domain Language
```go
// Unclear, generic naming
type Handler struct {
    // What does this handle? HTTP? Sockets? Rules?
}

func MatchValue(expected, actual any) bool {
    // What kind of matching? String? Object? Request?
}

type ProcessRules(request, config) {
    // What kind of processing? Evaluation? Execution?
}
```

## Consistency Guidelines

1. **Use domain terms consistently** - don't mix "rule" and "policy" or "socket" and "endpoint"
2. **Be specific** - "StringMatcher" not "Matcher"
3. **Express intent** - "EvaluateRules" not "ProcessRules"
4. **Use verbs for actions** - "CreateSocket", "DeleteSocket", "EvaluateRules"
5. **Use nouns for entities** - "Socket", "Rule", "Matcher", "Action"
