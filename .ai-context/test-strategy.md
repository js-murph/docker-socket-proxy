# Test Strategy

This document outlines the testing approach for the Docker Socket Proxy codebase.

## Testing Layers

### Unit Tests
**Scope**: Pure business logic, no I/O
**Location**: Same package as implementation
**Examples**: Domain matchers, modifiers, evaluators

```go
// Example: StringMatcher unit test
func TestUnit_StringMatcher_GivenExactMatch_WhenMatching_ThenReturnsTrue(t *testing.T) {
    matcher := NewStringMatcher("test")
    result := matcher.Matches("test")
    assert.True(t, result)
}
```

### Integration Tests
**Scope**: Service layer with real dependencies
**Location**: `*_integration_test.go` files
**Examples**: SocketService with in-memory repository

```go
// Example: SocketService integration test
func TestIntegration_SocketService_GivenValidConfig_WhenCreatingSocket_ThenSucceeds(t *testing.T) {
    repo := NewInMemorySocketRepository()
    service := NewSocketService(repo, logger)
    
    config := SocketConfig{Rules: []Rule{...}}
    socket, err := service.CreateSocket(ctx, config)
    
    assert.NoError(t, err)
    assert.NotNil(t, socket)
}
```

### E2E Tests
**Scope**: Full system with temporary directories
**Location**: `e2e/` directory
**Examples**: Complete proxy lifecycle

```go
// Example: E2E proxy test
func TestE2E_Proxy_GivenACLConfig_WhenRequestingDockerAPI_ThenEnforcesRules(t *testing.T) {
    // Start proxy server
    // Create socket with ACL rules
    // Make requests
    // Verify behavior
}
```

## Test Patterns

### Table-Driven Tests
Use for testing multiple scenarios with similar logic:

```go
func TestUnit_StringMatcher_GivenVariousPatterns_WhenMatching_ThenReturnsExpected(t *testing.T) {
    tests := []struct {
        name     string
        pattern  string
        input    string
        expected bool
    }{
        {"exact match", "test", "test", true},
        {"no match", "test", "other", false},
        {"regex match", "te.*", "test", true},
        {"regex no match", "te.*", "other", false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            matcher := NewStringMatcher(tt.pattern)
            result := matcher.Matches(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### Test Fixtures
Create reusable test data:

```go
// testfixtures/socket_config.go
func ValidSocketConfig() SocketConfig {
    return SocketConfig{
        Rules: []Rule{
            {
                Match: Match{
                    Path:   "/v1.*/containers/json",
                    Method: "GET",
                },
                Actions: []Action{
                    {Action: "allow"},
                },
            },
        },
    }
}

func DenyPrivilegedConfig() SocketConfig {
    return SocketConfig{
        Rules: []Rule{
            {
                Match: Match{
                    Path:   "/v1.*/containers/create",
                    Method: "POST",
                    Contains: map[string]any{
                        "HostConfig": map[string]any{
                            "Privileged": true,
                        },
                    },
                },
                Actions: []Action{
                    {Action: "deny", Reason: "Privileged containers not allowed"},
                },
            },
        },
    }
}
```

### Test Builders
For complex object construction:

```go
// testbuilders/socket_config_builder.go
type SocketConfigBuilder struct {
    config SocketConfig
}

func NewSocketConfigBuilder() *SocketConfigBuilder {
    return &SocketConfigBuilder{
        config: SocketConfig{Rules: []Rule{}},
    }
}

func (b *SocketConfigBuilder) WithAllowRule(path, method string) *SocketConfigBuilder {
    rule := Rule{
        Match: Match{Path: path, Method: method},
        Actions: []Action{{Action: "allow"}},
    }
    b.config.Rules = append(b.config.Rules, rule)
    return b
}

func (b *SocketConfigBuilder) WithDenyRule(path, method, reason string) *SocketConfigBuilder {
    rule := Rule{
        Match: Match{Path: path, Method: method},
        Actions: []Action{{Action: "deny", Reason: reason}},
    }
    b.config.Rules = append(b.config.Rules, rule)
    return b
}

func (b *SocketConfigBuilder) Build() SocketConfig {
    return b.config
}

// Usage in tests
config := NewSocketConfigBuilder().
    WithAllowRule("/v1.*/containers/json", "GET").
    WithDenyRule("/v1.*/containers/create", "POST", "Not allowed").
    Build()
```

### Interface-Based Mocking
Use interfaces for testability:

```go
// Mock repository for testing
type MockSocketRepository struct {
    sockets map[string]SocketConfig
    mu      sync.RWMutex
}

func NewMockSocketRepository() *MockSocketRepository {
    return &MockSocketRepository{
        sockets: make(map[string]SocketConfig),
    }
}

func (m *MockSocketRepository) Save(ctx context.Context, path string, config SocketConfig) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.sockets[path] = config
    return nil
}

func (m *MockSocketRepository) Load(ctx context.Context, path string) (SocketConfig, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    config, exists := m.sockets[path]
    if !exists {
        return SocketConfig{}, ErrNotFound
    }
    return config, nil
}
```

## Test Organization

### File Naming
- Unit tests: `*_test.go` in same package
- Integration tests: `*_integration_test.go`
- E2E tests: `e2e/*_test.go`

### Test Naming Convention
Pattern: `TestUnit_GivenX_WhenY_ThenZ` or `TestIntegration_Feature_Scenario`

Examples:
- `TestUnit_StringMatcher_GivenExactMatch_WhenMatching_ThenReturnsTrue`
- `TestIntegration_SocketService_GivenValidConfig_WhenCreatingSocket_ThenSucceeds`
- `TestE2E_Proxy_GivenACLConfig_WhenRequestingDockerAPI_ThenEnforcesRules`

### Test Structure
```go
func TestUnit_Component_GivenCondition_WhenAction_ThenExpected(t *testing.T) {
    // Arrange
    // Set up test data and mocks
    
    // Act
    // Execute the code under test
    
    // Assert
    // Verify the results
}
```

## Coverage Requirements

### Domain Layer
- **Target**: 100% coverage
- **Focus**: All edge cases and error conditions
- **Reason**: Pure logic, easy to test comprehensively

### Application Layer
- **Target**: 90% coverage
- **Focus**: Happy path and common error scenarios
- **Reason**: Integration with dependencies, some error paths hard to test

### Infrastructure Layer
- **Target**: 80% coverage
- **Focus**: Critical paths and error handling
- **Reason**: External dependencies, some scenarios hard to test

## Test Data Management

### Test Fixtures Directory
```
testfixtures/
├── socket_configs.go
├── rules.go
├── requests.go
└── responses.go
```

### Test Builders Directory
```
testbuilders/
├── socket_config_builder.go
├── rule_builder.go
└── request_builder.go
```

### Test Utilities
```go
// testutils/assertions.go
func AssertSocketEqual(t *testing.T, expected, actual Socket) {
    assert.Equal(t, expected.Path, actual.Path)
    assert.Equal(t, expected.Config, actual.Config)
    assert.Equal(t, len(expected.Rules), len(actual.Rules))
}

// testutils/helpers.go
func CreateTempSocketDir(t *testing.T) string {
    dir := t.TempDir()
    return filepath.Join(dir, "sockets")
}
```

## Running Tests

### Unit Tests Only
```bash
go test -short ./...
```

### Integration Tests
```bash
go test -tags=integration ./...
```

### E2E Tests
```bash
go test ./e2e/...
```

### All Tests
```bash
go test ./...
```

### Coverage
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Best Practices

1. **Test behavior, not implementation** - Focus on what the code does, not how
2. **Use descriptive test names** - Make it clear what's being tested
3. **Keep tests simple** - One assertion per test when possible
4. **Use table-driven tests** - For multiple similar scenarios
5. **Mock external dependencies** - Use interfaces for testability
6. **Test error conditions** - Don't just test happy paths
7. **Keep tests fast** - Unit tests should run in milliseconds
8. **Use test fixtures** - Reusable test data
9. **Clean up after tests** - Use `t.Cleanup()` for cleanup
10. **Write tests first** - TDD when refactoring
