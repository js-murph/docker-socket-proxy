# Architecture Decision Records

## ADR-001: Separate Matching from Evaluation

**Problem**: The current `matching.go` file mixes multiple concerns:
- Value comparison logic
- Regex pattern matching
- Request matching logic
- Body parsing and restoration

This makes the code hard to test, understand, and maintain.

**Decision**: Create separate interfaces and implementations:
- `ValueMatcher` interface for value comparison
- `RequestMatcher` interface for request matching
- `RuleEvaluator` for orchestrating rule evaluation
- `ActionExecutor` for executing actions

**Rationale**: 
- Single Responsibility Principle - each type has one clear purpose
- Easier to test individual components
- Clearer separation of concerns
- More maintainable and extensible

**Consequences**:
- More files and interfaces initially
- Clearer code organization
- Better testability
- Easier to add new matching strategies

## ADR-002: Repository Pattern for Storage

**Problem**: Current `FileStore` is tightly coupled to file system and mixed with business logic.

**Decision**: Create `SocketRepository` interface with implementations:
- `InMemorySocketRepository` for testing
- `FileSocketRepository` for production
- Repository handles only data persistence, not business logic

**Rationale**:
- Testability - can use in-memory implementation for tests
- Flexibility - can easily swap storage backends
- Single Responsibility - repository only handles data access
- Dependency Inversion - business logic depends on interface, not implementation

**Consequences**:
- More interfaces to maintain
- Clearer separation of data access from business logic
- Better testability
- Easier to add new storage backends

## ADR-003: Service Layer Pattern

**Problem**: HTTP handlers contain business logic, making them hard to test and reuse.

**Decision**: Create service layer with:
- `SocketService` for socket management operations
- `ProxyService` for proxy operations
- HTTP handlers become thin adapters that call services

**Rationale**:
- Separation of concerns - HTTP handling vs business logic
- Testability - can test services without HTTP
- Reusability - services can be used by CLI, HTTP, or other interfaces
- Single Responsibility - handlers only handle HTTP, services handle business logic

**Consequences**:
- More layers in the architecture
- Clearer separation of concerns
- Better testability
- Easier to add new interfaces (gRPC, etc.)

## ADR-004: Dependency Injection over Context

**Problem**: Current code uses context to pass dependencies, which is fragile and unclear.

**Decision**: Use constructor injection for dependencies:
- Services receive dependencies in constructors
- No global state or context-based dependency passing
- Clear dependency relationships

**Rationale**:
- Explicit dependencies - easy to see what a type needs
- Testability - easy to inject mocks
- No hidden dependencies
- Clearer code structure

**Consequences**:
- More constructor parameters
- Clearer dependency relationships
- Better testability
- No hidden dependencies

## ADR-005: Domain-Driven Design Structure

**Problem**: Current package structure doesn't reflect business domain clearly.

**Decision**: Organize packages by domain and layer:
- `internal/domain/` - pure business logic
- `internal/application/` - use cases and orchestration
- `internal/infrastructure/` - external concerns
- `internal/interfaces/` - adapters

**Rationale**:
- Clear separation of concerns
- Domain logic is independent of infrastructure
- Easy to understand and navigate
- Follows Clean Architecture principles

**Consequences**:
- More packages initially
- Clearer code organization
- Better separation of concerns
- Easier to maintain and extend

## ADR-006: Interface-Based Testing

**Problem**: Current tests are tightly coupled to implementations and hard to maintain.

**Decision**: Use interface-based testing:
- Test against interfaces, not concrete types
- Use dependency injection for test doubles
- Table-driven tests for domain logic
- Separate unit, integration, and E2E tests

**Rationale**:
- Testability - can easily mock dependencies
- Maintainability - tests don't break when implementation changes
- Clarity - tests focus on behavior, not implementation
- Coverage - can test edge cases more easily

**Consequences**:
- More interfaces to maintain
- Better test coverage
- More maintainable tests
- Clearer test organization

## ADR-007: Modifier Interface Design

**Problem**: During implementation, we discovered that the modifier interface needed to return both the modified body and a boolean indicating whether changes were made.

**Decision**: The `Modifier` interface returns `(map[string]interface{}, bool)` where:
- The first return value is the modified body
- The second return value indicates whether any changes were made

**Rationale**:
- The evaluator needs to know if modifications occurred to decide whether to return a modified result
- This allows for efficient change detection without deep comparison
- Maintains immutability principles by returning a new/modified body
- Enables proper testing of modification behavior

**Consequences**:
- Slightly more complex interface signature
- Better change detection and testing capabilities
- Clearer semantics about what constitutes a "modification"
