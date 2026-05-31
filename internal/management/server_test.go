package management

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vlourme/go-proxy/internal/config"
)

func newTestServer() *Server {
	return &Server{
		appCfg: &config.Config{
			ListenPort:   8080,
			BindPrefixes: []string{"2001:db8::/48"},
			MaxTimeout:   30,
			IdleTimeout:  30,
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
			Management: config.ManagementConfig{
				Enabled:       true,
				ListenAddress: "127.0.0.1",
				Port:          9090,
				Token:         "test-token",
			},
		},
		version: "1.0.0",
		commit:  "abc123",
		date:    "2024-01-01",
	}
}

func TestHealthz(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestStatusValidToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if !strings.Contains(string(body), `"version":"1.0.0"`) {
		t.Fatalf("expected version in body: %s", string(body))
	}
}

func TestStatusMissingToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestStatusWrongToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestStatusMalformedAuthHeader(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestStatusNoSpaceAuthHeader(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearertest-token")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestConfigRedactsSecrets(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.handleConfig(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "test-token") {
		t.Fatalf("management token leaked in response")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	mgmt, ok := result["management"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected management object in response")
	}
	if _, exists := mgmt["token"]; exists {
		t.Fatalf("management.token should not be present in response")
	}
}

func TestNotFound(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"code":"not_found"`) {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestHealthzMissingToken(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestStatusMethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"code":"method_not_allowed"`) {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestIPv6ListenAddress(t *testing.T) {
	s := &Server{
		appCfg: &config.Config{
			ListenPort:   8080,
			BindPrefixes: []string{"2001:db8::/48"},
			MaxTimeout:   30,
			IdleTimeout:  30,
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
			Management: config.ManagementConfig{
				Enabled:       true,
				ListenAddress: "::1",
				Port:          0,
				Token:         "test-token",
			},
		},
		version: "1.0.0",
		commit:  "abc123",
		date:    "2024-01-01",
	}

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop(nil)

	addr := s.Addr()
	if !strings.HasPrefix(addr, "[::1]:") {
		t.Fatalf("expected IPv6 address to start with [::1]:, got %s", addr)
	}
}
