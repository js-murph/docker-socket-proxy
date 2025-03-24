# Rules Configuration

Rules define how Docker Socket Proxy handles incoming requests. Each rule consists of a match section and an actions section.

## Rule Structure

Each rule in your configuration file looks like this:

```yaml
- match:
    path: "/v1.*/containers/json"  # Regex pattern for API path
    method: "GET"                  # HTTP method
    contains:                      # Optional content matching
      Env:
        - "DEBUG=true"
  actions:
    - action: "allow"              # Action to take
      reason: "Allow listing containers" # Optional documentation
```

## Match Criteria

The `match` section determines when a rule applies:

| Field | Description | Required | Example |
|-------|-------------|----------|---------|
| `path` | Regex pattern for the API path | Yes | `/v1.*/containers/json` |
| `method` | HTTP method to match | No | `GET`, `POST`, `DELETE` |
| `contains` | Content matching for request body | No | See below |

The `path` field supports regular expressions to match Docker API endpoints. Common patterns include:

- `/v1.*/containers/json` - List containers
- `/v1.*/containers/create` - Create a container
- `/v1.*/images/json` - List images
- `/v1.*/volumes` - Volume operations

The `method` field specifies which HTTP method to match. Common methods include:

- `GET` - Retrieve information
- `POST` - Create resources or trigger actions
- `DELETE` - Remove resources
- `PUT` - Update resources

The `contains` field allows you to match based on the content of the request body. This is particularly useful for container creation requests where you want to match based on environment variables, volumes, or other container configuration.

Example of content matching:

```yaml
match:
  path: "/v1.*/containers/create"
  method: "POST"
  contains:
    Env:
      - "DEBUG=true"
    HostConfig:
      Privileged: true
```

## Actions

Each rule can have multiple actions. The actions are processed in order, allowing you to perform multiple operations on a single request.

### Allow Action

Allows the request to proceed:

```yaml
actions:
  - action: "allow"
    reason: "Allow listing containers"
```

### Deny Action

Denies the request and returns an error:

```yaml
actions:
  - action: "deny"
    reason: "Privileged containers are not allowed"
```

### Upsert Action

Adds or updates fields in the request:

```yaml
actions:
  - action: "upsert"
    update:
      Env:
        - "SECURE=true"
      HostConfig:
        ReadonlyRootfs: true
```

### Replace Action

Replaces matching fields in the request:

```yaml
actions:
  - action: "replace"
    contains:
      HostConfig:
        Privileged: true
    update:
      HostConfig:
        Privileged: false
```

The `contains` field specifies which fields to match, and the `update` field specifies the replacement values.

### Delete Action

Deletes matching fields from the request:

```yaml
actions:
  - action: "delete"
    contains:
      Env:
        - "SECRET_.*"
```

The `contains` field supports regular expressions for matching array elements like environment variables.

## Processing Order

Rules are processed sequentially in the order they appear in the configuration file. For each rule:

1. The request is checked against the `match` criteria
2. If the match succeeds, the `actions` are applied in order
3. If an action is `allow` or `deny`, rule processing stops
4. Otherwise, processing continues with the next rule

## Examples

### Deny Privileged Containers

```yaml
- match:
    path: "/v1.*/containers/create"
    method: "POST"
    contains:
      HostConfig:
        Privileged: true
  actions:
    - action: "deny"
      reason: "Privileged containers are not allowed"
```

### Force Read-Only Root Filesystem

```yaml
- match:
    path: "/v1.*/containers/create"
    method: "POST"
  actions:
    - action: "upsert"
      update:
        HostConfig:
          ReadonlyRootfs: true
```

### Remove Sensitive Environment Variables

```yaml
- match:
    path: "/v1.*/containers/create"
    method: "POST"
  actions:
    - action: "delete"
      contains:
        Env:
          - "AWS_SECRET_.*"
          - "PASSWORD=.*"
```

### Add Required Labels

```yaml
- match:
    path: "/v1.*/containers/create"
    method: "POST"
  actions:
    - action: "upsert"
      update:
        Labels:
          socket-proxy: "docker-socket-proxy"
```

### Default Deny Rule

```yaml
- match:
    path: "/.*"
    method: ".*"
  actions:
    - action: "deny"
      reason: "Deny all other requests, the default is to allow everything"
```
