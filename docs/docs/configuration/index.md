# Configuration Overview

Docker Socket Proxy uses YAML configuration files to define how proxy sockets behave. This page provides an overview of the configuration structure and options.

## Basic Structure

A configuration file has two main sections:

```yaml
config:
  propagate_socket: "/var/run/docker.sock"

rules:
  - match:
      path: "/v1.*/volumes"
      method: "GET"
    actions:
      - action: "deny"
        reason: "Listing volumes is restricted"
```

## Config Section

The `config` section contains global settings for the proxy socket:

| Option | Description | Required | Default |
|--------|-------------|----------|---------|
| `propagate_socket` | Path to the Docker socket to proxy | No | - |

## Rules Section

The `rules` section is contains a list of rules that impose modifications or restrictions on the requests to the Docker socket. Each rule is processed sequentially and has a `match` section and an `actions` section.

The match section is used to determine if the rule should be applied to the request. The actions section is used to modify the request or respond to the request. Each action in a rule is processed sequentially and the order of the actions is important.

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

All match fields will attempt to match the entire string first and then attempt regex matching. The matching is done against the payload of the docker API request, so for available fields for matching see the [Docker API documentation](https://docs.docker.com/engine/api/).

### Actions

Below is a list of the currently supported actions:

| Action | Description |
|--------|-------------|
| `allow` | Stop processing the rule list and immediately allow the request to proceed |
| `deny` | Stop processing the rule list and immediately deny the request and return an error |
| `upsert` | Add or replace fields in the request |
| `replace` | Replace matching fields in the request |
| `delete` | Delete matching fields from the request |

For the `allow` and `deny` actions, you can provide a `reason` field for documentation:

```yaml
action: "deny"
reason: "Privileged containers are not allowed"
```

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
  - match:
      path: "/v1.*/volumes"
      method: "GET"
    actions:
      - action: "deny"
        reason: "Listing volumes is restricted"

  - match:
      path: "/v1.*/containers/create"
      method: "POST"
      contains:
        Env:
          - "BLOCK=true"
    actions:
      - action: "deny"
        reason: "Blocked creation of containers with restricted env variables"

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

  - match:
      path: "/.*"
      method: ".*"
    actions:
      - action: "deny"
        reason: "Deny all other requests, the default is to allow everything"
```

For more detailed information about rules, see the [Rules](rules.md) page.
