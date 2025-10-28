# Docker Socket Proxy Refactoring Plan

## Overview

Transform the codebase from AI-generated mess to maintainable, well-structured Go application following clean architecture principles. All refactoring will be behavior-preserving with comprehensive tests.

## Phase 1: Documentation & Foundation

### 1.1 Create Cursor Rules

Create `.cursorrules` file optimized for this Go project:

- Clean architecture principles (domain, application, infrastructure layers)
- Go idioms and conventions (error handling, naming, package structure)
- Testing guidelines (table-driven tests, mocking strategy, test organization)
- Domain language (Socket, Rule, Matcher, Action, Handler vs current unclear naming)
- Explicit guidance on separation of concerns
- No God objects, prefer composition over inheritance
- Context propagation patterns

### 1.2 Document Current Behavior

Create behavior preservation checklist:

- Socket creation/deletion/listing/describing
- ACL enforcement (allow/deny with path/method/body matching)
- Request rewriting (upsert/replace/delete operations)
- Configuration persistence and reload
- Signal handling and graceful shutdown
- Socket permission management

## Phase 2: Establish Clean Domain Model

### 2.1 Define Core Domain Types

Create `internal/domain/` package with:

- **Rule Engine**: `RuleMatcher`, `RuleEvaluator`, `ActionExecutor` (separate matching from evaluation from execution)
- **Value Matching**: `ValueMatcher` interface with concrete implementations (`StringMatcher`, `RegexMatcher`, `ArrayMatcher`, `ObjectMatcher`)
- **Request Modification**: `RequestModifier` interface with implementations (`UpsertModifier`, `ReplaceModifier`, `DeleteModifier`)
- **Socket Configuration**: Clean value objects for config representation

**Problem it solves**: Current `matching.go` has unclear responsibility - mixing value comparison, regex matching, and request matching. Name like "MatchValue" is ambiguous.

### 2.2 Extract Business Logic

Move from `internal/proxy/config/`:

- `matching.go` logic → domain matchers (properly named)
- `rewriting.go` logic → domain modifiers (properly named)
- Keep `conf.go` as config parsing/validation only

## Phase 3: Refactor Server Layer

### 3.1 Separate HTTP Transport from Business Logic

Current problems in `server/`:

- `ManagementHandler` creates sockets, manages lifecycle, AND handles HTTP
- `ProxyHandler` mixes ACL checking with request proxying
- Context-based dependency passing is fragile

Refactor to:

- **Service Layer**: `SocketService` (create/delete/list/describe operations)
- **HTTP Handlers**: Thin adapter calling service methods
- **Proxy Middleware**: Separate ACL/rewriting concerns from proxying

### 3.2 Improve Dependency Management

Replace:

- Context-based `Server` passing → explicit constructor injection
- `sync.RWMutex` sharing → encapsulate within repository
- Global state → proper dependency injection

Create:

- `SocketRepository` interface (implementations: in-memory, file-backed)
- `ProxyService` (orchestrates rule evaluation and proxying)
- Clean separation of read/write concerns

## Phase 4: Testing Strategy & Core Tests

### 4.1 Testing Architecture

Establish testing layers:

- **Unit tests**: Domain logic (matchers, modifiers) - pure functions, no I/O
- **Integration tests**: Service layer - use in-memory implementations
- **E2E tests**: Full server lifecycle - use temporary directories

### 4.2 Test Patterns

Document patterns in `.cursorrules`:

- Table-driven tests for domain logic
- Test fixtures/builders for complex config setup
- Interface-based mocking (no magic mocking libraries)
- Clear test names: `TestUnit_GivenX_WhenY_ThenZ`

### 4.3 Write Core Test Suites

You'll write these based on guidance:

**Rule Matching Tests** (`domain/matcher_test.go`):

```go
// Test string matching: exact, regex, in-array
// Test object matching: nested, partial, deep
// Test array matching: contains, all-match, partial
```

**Rule Evaluation Tests** (`domain/evaluator_test.go`):

```go
// Test rule precedence (first-match wins)
// Test action evaluation (allow/deny with conditions)
// Test body modification preservation
```

**Socket Service Tests** (`application/socket_service_test.go`):

```go
// Test CRUD operations
// Test concurrent access
// Test error scenarios
```

**E2E Tests** (`e2e/proxy_test.go`):

```go
// Test full proxy flow with ACLs
// Test configuration persistence
// Test graceful shutdown
```

## Phase 5: Incremental Refactoring

Execute in this order (minimize change blast radius):

1. **Create domain package** - extract pure logic, no dependencies
2. **Create application layer** - service interfaces and implementations
3. **Refactor storage** - implement repository pattern properly
4. **Refactor handlers** - make them thin adapters
5. **Refactor main.go** - clean dependency wiring
6. **Update CLI** - decouple from HTTP implementation details

Each step:

- Write tests first (or extract existing test logic)
- Refactor implementation
- Verify ALL existing tests still pass
- Commit

## Phase 6: Polish & Documentation

### 6.1 Code Quality

- Fix all linter warnings
- Add package documentation
- Consistent error messages
- Proper logging levels

### 6.2 Architecture Documentation

Create `docs/architecture.md`:

- Layer diagram
- Component responsibilities
- Key interfaces
- Testing approach

## Preserving Context for AI Tools

Create `.ai-context/` directory with:

**`refactoring-plan.md`**: This plan document

**`architecture-decisions.md`**: Record key decisions:

```markdown
# Architecture Decision Records

## ADR-001: Separate Matching from Evaluation
Problem: matching.go mixed concerns...
Decision: Create separate matcher interfaces...
Rationale: Easier to test, clearer responsibilities...
```

**`domain-language.md`**: Glossary of terms:

```markdown
- Socket: A proxy endpoint with associated rules
- Rule: Combination of matcher + actions
- Matcher: Determines if request matches criteria
- Action: What to do when rule matches (allow/deny/modify)
- Modifier: How to transform request body
```

**`test-strategy.md`**: Testing guidelines with examples

Update `.cursorrules` to reference these:

```
When working on this codebase, refer to:
- .ai-context/architecture-decisions.md for design rationale
- .ai-context/domain-language.md for correct terminology
- .ai-context/test-strategy.md for testing patterns
```

This way, any AI tool (Cursor, GitHub Copilot, etc.) can access this context.

## Key Files to Refactor (Priority Order)

1. `internal/proxy/config/matching.go` → `internal/domain/matcher/`
2. `internal/proxy/config/rewriting.go` → `internal/domain/modifier/`
3. `internal/server/proxy_handler.go` → thin adapter + `internal/application/proxy_service.go`
4. `internal/server/management_handler.go` → thin adapter + `internal/application/socket_service.go`
5. `internal/storage/file.go` → `internal/infrastructure/repository/`
6. `cmd/main.go` → proper dependency injection

## Success Criteria

- All existing tests pass
- New domain tests have 100% coverage
- No package cycles
- Clear separation: domain → application → infrastructure → interfaces
- Any developer (or AI) can understand the codebase structure in <5 minutes
