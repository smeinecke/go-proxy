package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vlourme/go-proxy/internal/config"
	"github.com/vlourme/go-proxy/internal/handlers"
	"github.com/vlourme/go-proxy/internal/management"
)

func getFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func setupProxyConfig() *config.Config {
	return &config.Config{
		ListenPort:     8080,
		BindPrefixes:   []string{"127.0.0.1/32"},
		DebugMode:      true,
		NetworkType:    "tcp",
		MaxTimeout:     30,
		IdleTimeout:    2,
		EnableFallback: false,
		Auth: struct {
			Type        config.AuthType `yaml:"type"`
			Credentials struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"credentials"`
			Redis struct {
				DSN string `yaml:"dsn"`
			} `yaml:"redis"`
		}{Type: config.AuthTypeNone},
		Management: config.ManagementConfig{Enabled: false},
	}
}

func setupFullConfig(t *testing.T) *config.Config {
	cfg := setupProxyConfig()
	cfg.Management = config.ManagementConfig{
		Enabled:       true,
		ListenAddress: "127.0.0.1",
		Port:          getFreePort(t),
		Token:         "integration-test-token",
	}
	return cfg
}

func TestManagementAPIReachable(t *testing.T) {
	cfg := setupFullConfig(t)
	if err := config.SetTestConfig(cfg); err != nil {
		t.Fatalf("failed to set test config: %v", err)
	}

	server := management.New(cfg, "test-version", "test-commit", "test-date")
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start management server: %v", err)
	}
	defer server.Stop(context.Background())

	baseURL := "http://" + server.Addr()
	client := &http.Client{Timeout: 5 * time.Second}

	// /healthz should require auth (all endpoints require auth in current design)
	resp, err := client.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated healthz, got %d", resp.StatusCode)
	}

	// /api/v1/status with valid token
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer integration-test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated status, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	if status["status"] != "ok" {
		t.Fatalf("unexpected status: %v", status["status"])
	}
	if status["version"] != "test-version" {
		t.Fatalf("unexpected version: %v", status["version"])
	}

	// /api/v1/config with valid token
	req, _ = http.NewRequest(http.MethodGet, baseURL+"/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer integration-test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("config request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated config, got %d", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, "integration-test-token") {
		t.Fatalf("management token leaked in config response")
	}

	// unknown route should return 404 JSON
	req, _ = http.NewRequest(http.MethodGet, baseURL+"/unknown", nil)
	req.Header.Set("Authorization", "Bearer integration-test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("unknown route request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", resp.StatusCode)
	}
}

func TestProxyHTTPForwarding(t *testing.T) {
	cfg := setupProxyConfig()
	if err := config.SetTestConfig(cfg); err != nil {
		t.Fatalf("failed to set test config: %v", err)
	}

	// Start a simple origin HTTP server.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/test" {
			w.Write([]byte("hello from origin"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer origin.Close()

	// Start the proxy listener.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handlers.HandleConnection(0, conn)
		}
	}()

	proxyAddr := listener.Addr().String()
	originURL := origin.URL + "/test"

	// Connect to the proxy and send an HTTP request.
	proxyConn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer proxyConn.Close()

	fmt.Fprintf(proxyConn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", originURL, listener.Addr().String())

	reader := bufio.NewReader(proxyConn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read proxy response: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if string(body) != "hello from origin" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestProxyAndManagementTogether(t *testing.T) {
	cfg := setupFullConfig(t)
	if err := config.SetTestConfig(cfg); err != nil {
		t.Fatalf("failed to set test config: %v", err)
	}

	// Start origin server.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxy works"))
	}))
	defer origin.Close()

	// Start proxy listener.
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go handlers.HandleConnection(0, conn)
		}
	}()

	// Start management server.
	server := management.New(cfg, "1.0.0", "abc", "2024-01-01")
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start management server: %v", err)
	}
	defer server.Stop(context.Background())

	proxyAddr := proxyListener.Addr().String()
	mgmtURL := "http://" + server.Addr()

	// Verify management API is reachable.
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, mgmtURL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer integration-test-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("management status request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from management, got %d", resp.StatusCode)
	}

	// Verify proxy is reachable and forwards traffic.
	proxyConn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer proxyConn.Close()

	fmt.Fprintf(proxyConn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", origin.URL, proxyAddr)

	reader := bufio.NewReader(proxyConn)
	resp, err = http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("failed to read proxy response: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from proxy, got %d", resp.StatusCode)
	}
	if string(body) != "proxy works" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}
