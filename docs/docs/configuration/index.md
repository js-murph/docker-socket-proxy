# Configuration Overview

Docker Socket Proxy uses YAML configuration files to define how proxy sockets behave. This page provides an overview of the configuration structure and options.

## Basic Structure

A configuration file has two main sections:

```yaml
config:
  propagate_socket: "/var/run/docker.sock"

rules:
  acls:
    - match:
        path: "/v1.*/containers/json"
        method: "GET"
      action: "allow"
      reason: "Allow listing containers"
  
  rewrites:
    - match:
        path: "/v1.*/containers/create"
        method: "POST"
      actions:
        - action: "upsert"
          update:
            Env:
              - "SECURE=true"
```

## Config Section

The `config` section contains global settings for the proxy socket:

| Option | Description | Required | Default |
|--------|-------------|----------|---------|
| `propagate_socket` | Path to the Docker socket to proxy | Yes | - |

## Rules Section

The `rules` section is divided into two subsections:

1. `acls`: Access control rules that determine whether requests are allowed or denied
2. `rewrites`: Rules for modifying request content before it's sent to Docker

### ACLs

The `acls` section contains a list of rules that control access to Docker API endpoints. Each rule consists of:

1. A `match` section that determines when the rule applies
2. An `action` field that specifies whether to allow or deny the request
3. An optional `reason` field for documentation

ACL rules are processed in order, and the first matching rule determines whether the request is allowed or denied.

Example ACL rule:

```yaml
acls:
  - match:
      path: "/v1.*/containers/json"
      method: "GET"
    action: "allow"
    reason: "Allow listing containers"
```

### Rewrites

The `rewrites` section contains rules for modifying request content before it's sent to Docker. Each rewrite rule consists of:

1. A `match` section that determines when the rule applies
2. An `actions` section that defines how to modify the request

Example rewrite rule:

```yaml
rewrites:
  - match:
      path: "/v1.*/containers/create"
      method: "POST"
    actions:
      - action: "upsert"
        update:
          Env:
            - "SECURE=true"
```

### Match Criteria

The `match` section supports the following fields:

| Field | Description | Example |
|-------|-------------|---------|
| `path` | Regex pattern for the API path | `/v1.*/containers/json` |
| `method` | HTTP method to match | `GET`, `POST`, `DELETE` |
| `contains` | Content matching for request body | See below |

The `contains` field allows you to match based on the content of the request body. This is particularly useful for container creation requests where you want to match based on environment variables, volumes, or other container configuration.

Example of content matching:

```yaml
match:
  path: "/v1.*/containers/create"
  method: "POST"
  contains:
    Env:
      - "DEBUG=true"
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

### Rewrite Actions

Rewrite rules support three types of actions:

| Action | Description |
|--------|-------------|
| `upsert` | Add or update fields in the request |
| `replace` | Replace matching fields in the request |
| `delete` | Delete matching fields from the request |

#### Upsert Action

Adds or updates fields in the request:

```yaml
actions:
  - action: "upsert"
    update:
      Env:
        - "SECURE=true"
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

#### Delete Action

Deletes matching fields from the request:

```yaml
actions:
  - action: "delete"
    contains:
      Env:
        - "SECRET_.*"
```

## Complete Example

Here's a complete example configuration:

```yaml
config:
  propagate_socket: "/var/run/docker.sock"

rules:
  acls:
    - match:
        path: "/v1.*/volumes"
        method: "GET"
      action: "deny"
      reason: "Listing volumes is restricted"

    - match:
        path: "/v1.*/containers/create"
        method: "POST"
        contains:
          Env:
            - "BLOCK=true"
      action: "deny"
      reason: "Blocked creation of containers with restricted env variables"

    - match:
        path: "/.*"
        method: ".*"
      action: "allow"
      reason: "Allow all other requests, the default is to block everything"

  rewrites:
    - match:
        path: "/v1.*/containers/create"
        method: "POST"
      actions:
        - action: "upsert"
          update:
            Env:
              - "FUN=yes"
        - action: "replace"
          contains:
            Env:
              - "DEBUG=true"
          update:
            Env:
              - "DEBUG=false"
        - action: "replace"
          contains:
            HostConfig:
              Privileged: true
          update:
            HostConfig:
              Privileged: false
        - action: "delete"
          contains:
            Env:
              - "PANTS=.*"
```

For more detailed information about rules, see the [Rules](rules.md) page.
