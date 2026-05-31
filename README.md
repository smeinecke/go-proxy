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
- Server with IPv6 support and large-enough subnet ([Hetzner](https://hetzner.cloud/?ref=BV2rohR8OBWQ) offers /64 subnets, *sponsored link*).

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


## Benchmark

Benchmark can be run with `go test -bench=.`. Configuration should have `test_port` set to 8081 and credentials
set to `username:password`.

**Benchmark results (AMD Ryzen 7 5800X, 8 cores / 16 threads):**
```sh
$ go run ./cmd/test/
Running benchmark: 100 concurrency, 600 total requests
Fastest:  295.753µs
Slowest:  11.004797ms
Average:  4.043747ms
Total:    26.447877ms
Throughput: 22686.13 req/s

Running benchmark: 250 concurrency, 1500 total requests
Fastest:  278.033µs
Slowest:  26.636579ms
Average:  7.914034ms
Total:    51.060407ms
Throughput: 29376.97 req/s

Running benchmark: 500 concurrency, 4000 total requests
Fastest:  244.612µs
Slowest:  73.656097ms
Average:  14.75182ms
Total:    122.857717ms
Throughput: 32557.99 req/s

Running benchmark: 1000 concurrency, 6000 total requests
Fastest:  762.657µs
Slowest:  106.522048ms
Average:  30.499627ms
Total:    194.252712ms
Throughput: 30887.60 req/s

Running benchmark: 2500 concurrency, 10000 total requests
Fastest:  450.594µs
Slowest:  298.337685ms
Average:  67.126755ms
Total:    307.735267ms
Throughput: 32495.46 req/s

Running benchmark: 5000 concurrency, 20000 total requests
Fastest:  2.158011ms
Slowest:  355.505753ms
Average:  133.22096ms
Total:    574.539255ms
Throughput: 34810.50 req/s

Running benchmark: 10000 concurrency, 40000 total requests
Fastest:  2.154811ms
Slowest:  1.167153127s
Average:  287.193985ms
Total:    1.241005216s
Throughput: 32231.94 req/s
```