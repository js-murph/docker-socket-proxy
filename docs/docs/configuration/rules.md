# Rules Configuration

Rules define how Docker Socket Proxy handles incoming requests. The rules section is divided into two main parts: ACLs (Access Control Lists) and rewrites.

## Rule Structure

The rules section in your configuration file looks like this:

```yaml
rules:
  acls:
    # ACL rules for allowing or denying requests
  rewrites:
    # Rewrite rules for modifying requests
```

## ACL Rules

ACL rules determine whether requests are allowed or denied. They are processed in order, and the first matching rule determines the outcome.

### ACL Rule Structure

Each ACL rule consists of:

```yaml
- match:
    path: "/v1.*/containers/json"  # Regex pattern for API path
    method: "GET"                  # HTTP method
    contains:                      # Optional content matching
      Env:
        - "DEBUG=true"
  action: "allow"                  # "allow" or "deny"
  reason: "Allow listing containers" # Optional documentation
```

### ACL Match Criteria

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

### ACL Actions

ACL rules support two actions:

| Action | Description |
|--------|-------------|
| `allow` | Allow the request to proceed |
| `deny` | Deny the request and return an error |

For both actions, you can provide a `reason` field for documentation:

```yaml
action: "deny"
reason: "Privileged containers are not allowed"
```

## Rewrite Rules

Rewrite rules modify the content of requests before they're sent to Docker. They allow you to add, modify, or remove fields from the request body.

### Rewrite Rule Structure

Each rewrite rule consists of:

```yaml
- match:
    path: "/v1.*/containers/create"  # Regex pattern for API path
    method: "POST"                   # HTTP method
  actions:
    - action: "upsert"               # Type of modification
      update:                        # Fields to update
        Env:
          - "SECURE=true"
```

### Rewrite Match Criteria

Rewrite rules use the same match criteria as ACL rules (path, method, and optionally contains).

### Rewrite Actions

Each rewrite rule can have multiple actions. The actions are applied in order, allowing you to perform multiple modifications on a single request.

#### Upsert Action

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

#### Replace Action

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

#### Delete Action

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

1. ACL rules are processed first, in order
2. If the request is allowed by ACL rules, rewrite rules are processed
3. Rewrite rules are processed in order, and all matching rules are applied
4. The modified request is sent to Docker

## Examples

### Deny Privileged Containers

```yaml
acls:
  - match:
      path: "/v1.*/containers/create"
      method: "POST"
      contains:
        HostConfig:
          Privileged: true
    action: "deny"
    reason: "Privileged containers are not allowed"
```

### Force Read-Only Root Filesystem

```yaml
rewrites:
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
rewrites:
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
rewrites:
  - match:
      path: "/v1.*/containers/create"
      method: "POST"
    actions:
      - action: "upsert"
        update:
          Labels:
            managed-by: "docker-socket-proxy"
            created-at: "{{.Timestamp}}"
```

For more examples, see the [Examples](examples.md) page.
