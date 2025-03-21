# Getting Started

This guide will help you get started with Docker Socket Proxy.

## Prerequisites

- Docker installed and running
- Basic understanding of [Docker and its API](https://docs.docker.com/reference/api/engine/version/v1.48/#tag/Container)
- Administrative privileges (for creating and managing sockets)

## Installation

Grab the latest release from GitHub and move it to a directory in your PATH:

```bash
curl -sSL https://github.com/js-murph/docker-socket-proxy/releases/latest/download/docker-socket-proxy.tgz
tar -xzf docker-socket-proxy.tgz
mv docker-socket-proxy /usr/local/bin/docker-socket-proxy
```

## Running the Daemon

The Docker Socket Proxy daemon is the core service that manages proxy sockets. Start it with:

```bash
docker-socket-proxy daemon
```

This will:

- Create the socket directory at `/var/run/docker-proxy/`
- Start the management socket at `/var/run/docker-proxy/management.sock`
- Begin listening for socket creation/deletion requests

For production use, you may want to run the daemon as a systemd service. An example service file is provided in the repository at `examples/docker-socket-proxy.service`.

## Creating Your First Proxy Socket

1. Create a configuration file (e.g., `config.yaml`):

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
        - action: "delete"
          contains:
            Env:
              - "PANTS=.*"
```

2. Create the proxy socket:

```bash
docker-socket-proxy socket create -c config.yaml
```

The command will output the path to your new proxy socket, typically something like:

```
Socket created: /var/run/docker-proxy/socket-12345.sock
```

## Using the Proxy Socket

You can now use this socket with Docker CLI or any other Docker client:

```bash
docker -H unix:///var/run/docker-proxy/socket-12345.sock ps
```

Or with Docker Compose by setting the `DOCKER_HOST` environment variable:

```bash
DOCKER_HOST=unix:///var/run/docker-proxy/socket-12345.sock docker-compose up
```

## Managing Proxy Sockets

List all available proxy sockets:

```bash
docker-socket-proxy socket list
```

View the configuration of a specific socket:

```bash
docker-socket-proxy socket describe socket-12345.sock
```

Delete a socket when you no longer need it:

```bash
docker-socket-proxy socket delete socket-12345.sock
```

## Next Steps

Now that you have a basic proxy socket running, you can:

1. Learn more about [configuration options](configuration/index.md)
2. Explore advanced [rule configurations](configuration/rules.md)
3. See [examples](configuration/examples.md) for common use cases
4. Check the [CLI reference](cli-reference.md) for all available commands
