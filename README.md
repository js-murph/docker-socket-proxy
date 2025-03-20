# Docker Socket Proxy

A proxy for the Docker socket that allows for adding ACLs and rewriting requests and responses. This binary contains a server that runs the proxy and a CLI for managing it.

## Features

- **Multiple Socket Support**: Create multiple proxy sockets with different access control rules
- **Fine-grained ACLs**: Control access to Docker API endpoints based on path, method, and request content
- **Request Rewriting**: Modify Docker API requests on-the-fly (replace, upsert, or delete fields)
- **Command-line Interface**: Easy-to-use CLI for managing proxy sockets

## But Why?

Every now and then in this crazy life you just gotta do docker-in-docker every now and again, this helps you enforce some rules on the docker socket. This is _definitely not_ an appropriate replacement for a proper secure Docker runtime such as [sysbox](https://github.com/nestybox/sysbox).

But maybe if you are doing some docker-in-docker in CI and want to make sure that a particular mount is available on every subcontainer, then this might help you out. Or maybe you're running Traefik using the Docker socket and want to make sure it can only access particular endpoints, in that case we got you covered.

## Installation

Grab it from the releases page and move it to a directory in your PATH.

```bash
curl -sSL https://github.com/js-murph/docker-proxy-socket/releases/latest/download/docker-socket-proxy.tgz
tar -xzf docker-socket-proxy.tgz
mv docker-socket-proxy /usr/local/bin/docker-socket-proxy
```

There's an example configuration file in [examples/config.yaml](examples/config.yaml). See the [docs](https://js-murph.github.io/docker-proxy-socket/docs/configuration) for more information.

You can also find an example systemd service file in [examples/docker-socket-proxy.service](examples/docker-socket-proxy.service).

## Usage

To start the server run:

```bash
docker-socket-proxy daemon
```

To create a new proxy socket run:

```bash
docker-socket-proxy socket create -c /path/to/config.yaml
```

This will return a socket that you can use instead of the Docker socket with the rules applied.

## Local Development

First ensure you have [hermit installed](https://cashapp.github.io/hermit/#quickstart).

```bash
git clone https://github.com/js-murph/docker-socket-proxy.git
cd docker-socket-proxy
. bin/activate-hermit
```

To run the tests:

```bash
gotestsum
```

To build the binary:

```bash
go build -o docker-socket-proxy cmd/main.go
```
