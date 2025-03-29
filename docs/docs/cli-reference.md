# CLI Reference

Docker Socket Proxy provides a command-line interface for managing the proxy server and sockets. This reference documents all available commands and their options.

## Global Options

These options apply to all commands:

```
--help, -h      Show help for a command
```

## daemon

Starts the Docker Socket Proxy daemon. The daemon proxies requests to the Docker daemon and also provides a management socket so that it can be configured.

```bash
docker-socket-proxy daemon [flags]
```

### Options

```
--management-socket string   Path to the management socket (default "/var/run/docker-proxy/management.sock")
--docker-socket string       Path to the Docker daemon socket (default "/var/run/docker.sock")
```

### Example

```bash
# Start the daemon with default settings
docker-socket-proxy daemon

# Start the daemon with a custom Docker socket
docker-socket-proxy daemon --docker-socket /path/to/custom/docker.sock
```

## socket

Commands for managing proxy sockets.

```bash
docker-socket-proxy socket [command]
```

### Available Commands

- `create`: Create a new proxy socket
- `delete`: Delete an existing proxy socket
- `list`: List all available proxy sockets
- `describe`: Show details about a proxy socket

## socket create

Creates a new proxy socket with the specified configuration.

```bash
docker-socket-proxy socket create [flags]
```

### Options

```
--config, -c string   Path to socket configuration file (yaml)
--output              Output format, options are: yaml, json, text, silent (defaults to yaml)
```

### Example

```bash
# Create a new socket with a configuration file
docker-socket-proxy socket create -c /path/to/config.yaml
```

## socket delete

Deletes an existing proxy socket.

```bash
docker-socket-proxy socket delete [socket-path]
```

### Example

```bash
# Delete a socket by name
docker-socket-proxy socket delete my-socket.sock

# Delete a socket by full path
docker-socket-proxy socket delete /var/run/docker-proxy/my-socket.sock
```

## socket list

Lists all available proxy sockets.

```bash
docker-socket-proxy socket list
```

### Example

```bash
# List all sockets
docker-socket-proxy socket list
```

## socket describe

Shows detailed information about a proxy socket, including its configuration.

```bash
docker-socket-proxy socket describe [socket-name]
```

### Example

```bash
# Describe a socket
docker-socket-proxy socket describe my-socket.sock
```
