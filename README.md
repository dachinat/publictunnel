# PublicTunnel

<img height="175" src="https://publictunnel.com/logo-light.png" alt="PublicTunnel logo">

A lightweight tunneling service built in Golang - expose your local services to public.

## Features
- WebSocket control channel
- Dynamic subdomain allocation
- Concurrent request proxying
- Simple CLI for server and client

## Installation

```bash
go build -o server ./cmd/server
go build -o client ./cmd/client
```

## Client usage

### Start the Client
Connect your local development server to the public tunnel.

```bash
./client -port 8080 -subdomain my-app
```

This will make your local service at `http://localhost:8080` available at `http://my-app.publictunnel.com`.

## Architecture
- **Server**: Listens on port 4000. It handles WebSocket connections from clients and proxies incoming HTTP requests based on the `Host` header.
- **Client**: Connects to the server via WebSocket. When an HTTP request is received over the tunnel, it performs the request against the local port and returns the response.
- **Protocol**: Custom JSON messages over WebSocket for control and data.
