# Behavior Preservation Checklist

This document lists all current behaviors that must be preserved during refactoring. Each item should be verified after refactoring to ensure no functionality is lost.

## Socket Management

### Socket Creation
- [ ] **Create socket with UUID-based name**: `docker-proxy-{uuid}.sock`
- [ ] **Set socket permissions to 0660**: `os.Chmod(socketPath, 0660)`
- [ ] **Create Unix socket listener**: `net.Listen("unix", socketPath)`
- [ ] **Start proxy server in goroutine**: `go func() { server.Serve(listener) }()`
- [ ] **Track socket in createdSockets list**: `srv.TrackSocket(socketPath)`
- [ ] **Save configuration to file**: `store.SaveConfig(socketPath, socketConfig)`
- [ ] **Return socket path in JSON response**: `{"status": "success", "response": {"socket": "/path/to/socket"}}`
- [ ] **Handle empty config**: Default empty config if none provided
- [ ] **Validate JSON config**: Return 400 for invalid JSON
- [ ] **Handle socket creation errors**: Return 500 with error message

### Socket Deletion
- [ ] **Remove socket file**: `os.Remove(socketPath)`
- [ ] **Stop proxy server**: `server.Close()`
- [ ] **Remove from tracking**: `srv.UntrackSocket(socketPath)`
- [ ] **Delete config file**: `store.DeleteConfig(socketPath)`
- [ ] **Remove from config map**: `delete(socketConfigs, socketPath)`
- [ ] **Support query parameter**: `?socket=path` or `Socket-Path` header
- [ ] **Handle missing socket**: Return 404 if socket not found
- [ ] **Return success message**: `{"status": "success", "response": {"message": "Socket deleted"}}`
- [ ] **Continue on partial errors**: Log errors but continue cleanup

### Socket Listing
- [ ] **List all socket names**: Extract basename from socket paths
- [ ] **Return JSON array**: `{"status": "success", "response": {"sockets": ["name1", "name2"]}}`
- [ ] **Handle empty list**: Return empty array if no sockets
- [ ] **Thread-safe access**: Use RWMutex for config map access

### Socket Description
- [ ] **Return socket configuration**: Full SocketConfig object
- [ ] **Support query parameter**: `?socket=name`
- [ ] **Handle missing socket**: Return 404 if socket not found
- [ ] **Return JSON response**: `{"status": "success", "response": {"config": {...}}}`
- [ ] **Support text output**: YAML encoding for text format

### Socket Cleanup
- [ ] **Remove all sockets**: Delete all tracked sockets
- [ ] **Stop all servers**: Close all proxy servers
- [ ] **Clean up files**: Remove all socket and config files
- [ ] **Return count**: `{"status": "success", "message": "Deleted N sockets"}`
- [ ] **Handle partial failures**: Log errors but continue

## ACL Enforcement

### Rule Matching
- [ ] **Path matching with regex**: `regexp.MatchString(rule.Match.Path, r.URL.Path)`
- [ ] **Method matching with regex**: `regexp.MatchString(rule.Match.Method, r.Method)`
- [ ] **Body content matching**: Parse JSON body and match against `rule.Match.Contains`
- [ ] **Nested object matching**: Recursive matching for nested structures
- [ ] **Array matching**: Match elements in arrays
- [ ] **String matching**: Exact and regex string matching
- [ ] **First-match wins**: Stop at first matching rule
- [ ] **Default allow**: Allow if no rules match

### Action Execution
- [ ] **Allow action**: Allow request to proceed
- [ ] **Deny action**: Return 403 with reason
- [ ] **Action precedence**: Allow overrides deny in same rule
- [ ] **Rule precedence**: First matching rule wins
- [ ] **Body preservation**: Restore original body after processing
- [ ] **Content-Length update**: Update header when body modified

### Request Modification
- [ ] **Upsert action**: Add or update fields in request body
- [ ] **Replace action**: Replace existing fields
- [ ] **Delete action**: Remove fields from request body
- [ ] **Nested modification**: Support nested object updates
- **Array modification**: Support array element updates
- [ ] **Key-value array handling**: Special handling for `key=value` arrays
- [ ] **Body restoration**: Restore original body if no modifications

## Configuration Persistence

### File Storage
- [ ] **JSON format**: Store configs as JSON files
- [ ] **File naming**: `{socket-name}.json` in socket directory
- [ ] **Directory creation**: Create directories as needed
- [ ] **File permissions**: 0644 for config files
- [ ] **Atomic writes**: Write to temp file then rename
- [ ] **Error handling**: Continue on file errors

### Configuration Loading
- [ ] **Load on startup**: Load existing configs when server starts
- [ ] **Recreate sockets**: Create listeners for existing configs
- [ ] **Validate configs**: Validate loaded configurations
- [ ] **Skip invalid configs**: Log errors but continue
- [ ] **Set permissions**: Set socket permissions on recreation

### Configuration Validation
- [ ] **Required fields**: Validate required fields are present
- [ ] **Rule validation**: Validate each rule structure
- [ ] **Action validation**: Validate action types and parameters
- [ ] **Regex validation**: Validate regex patterns
- [ ] **Error messages**: Clear error messages for validation failures

## Signal Handling

### Graceful Shutdown
- [ ] **Signal handling**: Handle SIGINT and SIGTERM
- [ ] **Server shutdown**: Shutdown HTTP servers with timeout
- [ ] **Socket cleanup**: Remove all socket files
- [ ] **Config cleanup**: Remove config files
- [ ] **Timeout handling**: 5-second timeout for shutdown
- [ ] **Error logging**: Log shutdown errors

### Resource Cleanup
- [ ] **Close listeners**: Close all socket listeners
- [ ] **Stop goroutines**: Stop all background goroutines
- [ ] **Remove files**: Clean up all temporary files
- [ ] **Release locks**: Release all mutexes

## Socket Permissions

### Permission Management
- [ ] **Management socket**: 0660 permissions
- [ ] **Proxy sockets**: 0660 permissions
- [ ] **Config files**: 0644 permissions
- [ ] **Directories**: 0755 permissions
- [ ] **Error handling**: Log permission errors but continue

## Error Handling

### HTTP Error Responses
- [ ] **400 Bad Request**: Invalid JSON, missing parameters
- [ ] **403 Forbidden**: ACL denial
- [ ] **404 Not Found**: Socket not found
- [ ] **500 Internal Server Error**: Server errors
- [ ] **Error JSON format**: `{"status": "error", "response": {"error": "message"}}`

### Logging
- [ ] **Request logging**: Log all management requests
- [ ] **Error logging**: Log all errors with context
- [ ] **Debug logging**: Log debug information
- [ ] **Structured logging**: Use structured logging with fields

## CLI Integration

### Command Line Interface
- [ ] **Socket create**: `docker-socket-proxy socket create -c config.yaml`
- [ ] **Socket delete**: `docker-socket-proxy socket delete /path/to/socket`
- [ ] **Socket list**: `docker-socket-proxy socket list`
- [ ] **Socket describe**: `docker-socket-proxy socket describe socket-name`
- [ ] **Socket clean**: `docker-socket-proxy socket clean`
- [ ] **Output formats**: text, json, yaml, silent
- [ ] **Error handling**: Proper error messages and exit codes

### HTTP Client
- [ ] **Unix socket client**: Connect to management socket
- [ ] **Request formatting**: Proper HTTP requests
- [ ] **Response parsing**: Parse JSON responses
- [ ] **Error handling**: Handle connection and response errors

## Performance

### Concurrency
- [ ] **Thread-safe operations**: Use mutexes for shared state
- [ ] **Goroutine management**: Proper goroutine lifecycle
- [ ] **Lock ordering**: Avoid deadlocks
- [ ] **Resource cleanup**: Clean up goroutines on shutdown

### Memory Management
- [ ] **Body reading**: Read request body only once
- [ ] **Body restoration**: Restore body for downstream
- [ ] **Memory leaks**: Avoid memory leaks in long-running processes
- [ ] **Resource limits**: Reasonable limits on resources

## Testing

### Test Coverage
- [ ] **Unit tests**: All domain logic covered
- [ ] **Integration tests**: Service layer covered
- [ ] **E2E tests**: Full system covered
- [ ] **Error cases**: Error conditions tested
- [ ] **Edge cases**: Boundary conditions tested

### Test Data
- [ ] **Test fixtures**: Reusable test data
- [ ] **Test builders**: Complex object construction
- [ ] **Mock objects**: Interface-based mocking
- [ ] **Test isolation**: Tests don't interfere with each other

## Verification Process

After each refactoring step:

1. **Run all tests**: `go test ./...`
2. **Check specific behaviors**: Use this checklist
3. **Manual testing**: Test CLI commands
4. **Integration testing**: Test with real Docker daemon
5. **Performance testing**: Ensure no performance regression

## Notes

- All behaviors must be preserved exactly as they are
- No breaking changes to external APIs
- Maintain backward compatibility
- Preserve all error messages and status codes
- Keep all logging behavior
- Maintain all configuration options
