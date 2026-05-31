# Go Proxy Server

A configurable proxy server in Go, supporting rotating IPv4/IPv6 addresses, session and external authentication.

## Features

- IPv4 or IPv6 back-connect
- HTTP and SOCKS5(h) support
- Multiple IPv6-IPv4 prefixes supported, with per-country prefixes
- Session and timeout support to re-use generated IP
- Up to 14,000 requests per second
- DNS resolution and caching
- Automatic IPv6 routing and sysctl setup
- Authentication with Redis or credentials
- One listener per core via `SO_REUSEPORT` and `SO_REUSEADDR`

## Setup

### Prerequisites

- [Go](https://golang.org/dl/)
- Server with IPv6 support and large-enough subnet.

### Configuration

Copy the `config.example.yaml` file to `config.yaml` and modify the settings as needed.

Example:
```yaml
listen_address: "::"
listen_port: 8080
debug_mode: false # Enable pretty-print logs, don't enable in production
test_port: -1 # Enable a test server on port 8081 for benchmarking, -1 to disable
network_type: "tcp6" # tcp = dual-stack, tcp6 = IPv6 only, tcp4 = IPv4 only
max_timeout: 30 # Session cache TTL in minutes
idle_timeout: 30 # Tunnel idle timeout in seconds
auth:
  type: "credentials" # none, credentials, redis
  credentials:
    username: "username"
    password: "password"
  redis:
    dsn: "redis://localhost:6379" # Will compare the username as key to the password
bind_prefixes: # IPv4/IPv6 prefixes to bind to (must be byte-aligned, e.g., /48, /56, /64)
  - "2a14:dead:beef::1/48"
  - "2a14:dead:feed::1/48"
enable_fallback: true # Fallback to IPv4 if the target does not match generated IP family above
fallback_prefixes:
  - "1.2.3.4/32" # List of prefixes to fallback to, must be IPv4 and byte-aligned
located_prefixes:
  ch:
    - "2a14:dead:beef::/48"
  uk:
    - "2a14:dead:feed::/48"
replace_ips:
  "1.2.3.0/24": "2a14:dead:beef::"
deleted_headers: # List of headers to delete (HTTP only), this make proxy anonymous
  - "Proxy-Authorization"
  - "Proxy-Connection"
dns:
  type: "system" # system or custom
  servers:
    - "1.1.1.1"
  timeout: 5
blocked_cidrs: # Additional blocked CIDRs beyond default private/reserved ranges
  - "10.0.0.0/8"
```

### Build

```bash
go build .
./go-proxy
```

### Usage example

```bash
# Without credentials, if enabled
curl -x http://localhost:8080 http://api.ipquery.io

# Session
curl -x http://john-session-abcdef1234:doe@localhost:8080 http://api.ipquery.io

# Session with timeout
curl -x http://john-session-abcdef1234-timeout-10:doe@localhost:8080 http://api.ipquery.io

# Location override
curl -x http://john-country-ch:doe@localhost:8080 http://api.ipquery.io

# Disable fallback (disable switching to IPv4 if the target is not IPv6-capable)
# Recommended when using IPv6-only websites to ensure the IP is always IPv6
curl -x http://john-fallback-no:doe@localhost:8080 http://api.ipquery.io

# SOCKS5
curl -x socks5://john:doe@localhost:8080 http://api.ipquery.io

# SOCKS5h (server-side DNS resolution, recommended)
curl -x socks5h://john:doe@localhost:8080 http://api.ipquery.io
```

> Session ID must be alphanumeric, between 6 and 24 characters.
> The timeout is in minutes, between 1 and 30 minutes.

### IP Override

IP override is a map of CIDR to IP.
If the resolved IP is present in any defined CIDR, it will be replaced with the override.
Some domains does not reply any `AAAA` record, but in fact, they support IPv6 by replacing the DNS resolution.
This can be used to bypass some IPv6 limitations of CDN and DNS providers.

Example config:
```yaml
replace_ips:
  "1.2.3.0/24": "2a14:dead:beef::"
```

If you're domain resolve to `1.2.3.4`, it will be replaced with `2a14:dead:beef::` by the proxy.


## Management API

The management API is an optional authenticated HTTP API for monitoring and administration. It is **disabled by default** and runs on a separate port from the proxy.

### Configuration

```yaml
management:
  enabled: false
  listen_address: "127.0.0.1"
  port: 9090
  token: "change-me"
```

- `enabled`: whether the management API is started (`false` by default)
- `listen_address`: address to bind the management server (`127.0.0.1` by default)
- `port`: port to listen on
- `token`: Bearer token required for authentication

**Security notes:**
- The management API is disabled by default.
- It runs on a separate port from the proxy.
- A token is required when enabled.
- By default, binding to `0.0.0.0` or `::` is rejected. Set `allow_public: true` to override.
- It should be bound to localhost or a private admin network.
- It is not intended to be exposed publicly without additional protection.

### Example request

```bash
curl -H "Authorization: Bearer change-me" http://127.0.0.1:9090/api/v1/status
```

### Endpoints

- `GET /healthz` - health check (requires authentication)
- `GET /api/v1/status` - server status and version metadata
- `GET /api/v1/config` - safe non-secret configuration values

## Pre-created sessions for browsers

Chrome and other browsers cannot send custom proxy headers. To bind a specific outgoing source IP for a browser session, pre-create the session through the Management API and then use the returned `proxy_username` as the normal proxy credential.

### Creating a session

```bash
curl -X POST http://127.0.0.1:9090/api/v1/sessions \
  -H "Authorization: Bearer change-me" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john",
    "session": "abc123",
    "source_ip": "2001:db8::1234",
    "ttl_minutes": 30
  }'
```

Response:
```json
{
  "username": "john",
  "session": "abc123",
  "source_ip": "2001:db8::1234",
  "location": "",
  "fallback": "no",
  "proxy_username": "john-session-abc123-fallback-no",
  "expires_at": "2026-05-31T20:00:00Z"
}
```

### Using the session in Chrome

Configure Chrome proxy credentials using the exact `proxy_username` returned by the API:
- **username**: `john-session-abc123-fallback-no`
- **password**: `<proxy password>`

No custom headers are required. The proxy parses the session from the username and uses the pre-created source IP. Always use the returned `proxy_username` verbatim.

### Rules

- `source_ip` must be inside the configured proxy pools (`bind_prefixes`, `located_prefixes`, or `fallback_prefixes`).
- The Management API is disabled by default.
- The Management API must be bound to localhost or a private admin network.
- If the session is omitted, a random 12-character session ID is generated automatically.
- Pre-created sessions override random IP generation for that session key.
- Existing random IPv6 rotation is unchanged when no pre-created session exists.
- Duplicate sessions return `409 Conflict` unless `overwrite: true` is sent.
- Pre-created sessions default `fallback` to `no` for exact-IP semantics. The only accepted values are `no` and `yes`.

## Benchmarks

### Microbenchmarks

Run microbenchmarks for hot paths with memory profiling:

```bash
go test -bench=. -benchmem ./internal/...
```

Benchmarked paths:
- `BenchmarkParseRequest` - HTTP request parsing
- `BenchmarkWriteTo` - request forwarding serialization
- `BenchmarkGetCredentials` - auth extraction from headers
- `BenchmarkSplitParams` / `BenchmarkGetParams` - username parameter parsing
- `BenchmarkMakeSessionKey` - session key generation
- `BenchmarkSessionStoreGet` / `BenchmarkSessionStoreSet` - session cache
- `BenchmarkRouterRouteNewIPv6` / `BenchmarkRouterRouteExistingSession` - routing
- `BenchmarkResolverCachedLookup` / `BenchmarkResolverBlockedCheck` - DNS/resolver

### End-to-end Load Benchmark

`cmd/bench` runs duration-based load tests against a live proxy:

```bash
# Start proxy with test_port enabled
go run . &

# HTTP small requests
go run ./cmd/bench \
  -mode http-small \
  -proxy http://username:password@127.0.0.1:8080 \
  -target http://[::1]:8081 \
  -concurrency 1000 \
  -warmup 10s \
  -duration 60s

# CONNECT (HTTPS tunneling)
go run ./cmd/bench \
  -mode connect-small \
  -proxy http://username:password@127.0.0.1:8080 \
  -target "[::1]:8081" \
  -concurrency 1000 \
  -duration 60s

# Large body (64KB) HTTP POST
go run ./cmd/bench -mode http-large ...

# Sticky session (reuse same session ID)
go run ./cmd/bench -mode sticky-session ...

# Random IP rotation (new session per request)
go run ./cmd/bench -mode random-ip ...
```

Modes:
- `http-small` - GET requests, small response
- `http-large` - POST requests with 64KB body
- `connect-small` - CONNECT tunnel establishment
- `connect-large` - CONNECT with larger payload
- `sticky-session` - reuse same session to test cache hit path
- `random-ip` - force new IP generation per request

Reported metrics:
- total requests
- success / error counts
- throughput (req/s)
- p50, p95, p99 latency
- max latency

### Comparing branches

Use `benchstat` to compare microbenchmarks across branches:

```bash
# On old branch
go test -bench=. -count=6 ./internal/... > old.txt

# On new branch
go test -bench=. -count=6 ./internal/... > new.txt

# Compare
go install golang.org/x/perf/cmd/benchstat@latest
benchstat old.txt new.txt
```

For end-to-end comparison, run `cmd/bench` multiple times on each branch and compare medians:

```bash
for i in 1 2 3; do
  go run ./cmd/bench -mode http-small -duration 30s 2>/dev/null | grep "Throughput"
done
```