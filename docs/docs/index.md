# Docker Socket Proxy

A secure proxy for Docker socket with fine-grained access control and request rewriting capabilities.

## Features

- **Multiple Socket Support**: Create multiple proxy sockets with different access control rules
- **Fine-grained ACLs**: Control access to Docker API endpoints based on path, method, and request content
- **Request Rewriting**: Modify Docker API requests on-the-fly (replace, upsert, or delete fields)
- **Command-line Interface**: Easy-to-use CLI for managing proxy sockets

## But Why?

Every now and then against your better judgement you may find yourself doing docker-in-docker, this helps you enforce some rules on the docker socket.

Maybe you are doing some docker-in-docker in CI and want to make sure that a particular mount is available on every subcontainer. Or maybe you're running Traefik using the Docker socket and want to make sure it can only access particular endpoints, in that case we got you covered.

This is _definitely not_ an appropriate replacement for a secure Docker runtime such as [sysbox](https://github.com/nestybox/sysbox).

## Installation

Grab it from the releases page and move it to a directory in your PATH.

```bash
curl -sSL https://github.com/js-murph/docker-socket-proxy/releases/latest/download/docker-socket-proxy.tgz
tar -xzf docker-socket-proxy.tgz
mv docker-socket-proxy /usr/local/bin/docker-socket-proxy
```

## Quick Start

To start the server run:

```bash
docker-socket-proxy daemon
```

To create a new proxy socket run:

```bash
docker-socket-proxy socket create -c /path/to/config.yaml
```

This will return a socket that you can use instead of the Docker socket with the rules applied.

See the [Getting Started](getting-started.md) section for more details.

## Configuration Example

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
      path: "/.*"
      method: ".*"
    actions:
      - action: "allow"
        reason: "Allow all other requests, the default is to block everything"
```

See the [Configuration](configuration/index.md) section for more details.
