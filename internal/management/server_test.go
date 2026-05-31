package management

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/vlourme/go-proxy/internal/config"
	"github.com/vlourme/go-proxy/internal/routing"
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
			EnableFallback: true,
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

func newTestServerWithRouter() *Server {
	s := newTestServer()
	s.sessionStore = routing.NewSessionStore(1024)

	bindPrefix := netip.MustParsePrefix("2001:db8::/48")
	fallbackPrefix := netip.MustParsePrefix("203.0.113.0/24")
	locatedPrefixes := map[string][]netip.Prefix{
		"uk": {netip.MustParsePrefix("2001:db8:1::/48")},
	}

	s.router = routing.NewRouter(
		s.sessionStore,
		[]netip.Prefix{bindPrefix},
		[]netip.Prefix{fallbackPrefix},
		locatedPrefixes,
		30*time.Minute,
		true,
	)
	blockedPrefixes := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	s.resolver = routing.NewResolver("system", nil, 5*time.Second, true, blockedPrefixes, nil)
	return s
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

func postSessions(t *testing.T, s *Server, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)
	return w.Result()
}

func TestSessionsMissingToken(t *testing.T) {
	s := newTestServerWithRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}
}

func TestSessionsInvalidToken(t *testing.T) {
	s := newTestServerWithRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	s.buildRouter().ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Result().StatusCode)
	}
}

func TestSessionsMalformedJSON(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{bad json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsMissingUsername(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"source_ip": "2001:db8::1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsMissingSourceIP(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsInvalidSourceIP(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "not-an-ip"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsSourceIPWithPort(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "127.0.0.1:8080"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsSourceIPOutsidePool(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db9::1"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestSessionsIPv6InsideBindPrefix(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456"}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["source_ip"] != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %v", result["source_ip"])
	}
	if result["proxy_username"] != "john-session-abc123456" {
		t.Fatalf("expected john-session-abc123456, got %v", result["proxy_username"])
	}
}

func TestSessionsIPv6LocatedPrefix(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8:1::1", "session": "abc123456", "location": "uk"}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestSessionsIPv6WrongLocation(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456", "location": "uk"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestSessionsUnknownLocation(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456", "location": "fr"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestSessionsIPv4FallbackEnabled(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "203.0.113.1", "session": "abc123456"}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestSessionsIPv4FallbackDisabled(t *testing.T) {
	s := newTestServerWithRouter()
	s.appCfg.EnableFallback = false
	resp := postSessions(t, s, `{"username": "john", "source_ip": "203.0.113.1", "session": "abc123456"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestSessionsIPv4OutsideFallback(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "10.0.0.1", "session": "abc123456"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestSessionsDuplicate(t *testing.T) {
	s := newTestServerWithRouter()
	body := `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456"}`
	resp1 := postSessions(t, s, body)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first request expected 201, got %d", resp1.StatusCode)
	}
	resp2 := postSessions(t, s, body)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp2.StatusCode)
	}
}

func TestSessionsDuplicateOverwrite(t *testing.T) {
	s := newTestServerWithRouter()
	body := `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456"}`
	resp1 := postSessions(t, s, body)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first request expected 201, got %d", resp1.StatusCode)
	}
	body2 := `{"username": "john", "source_ip": "2001:db8::2", "session": "abc123456", "overwrite": true}`
	resp2 := postSessions(t, s, body2)
	if resp2.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 201, got %d: %s", resp2.StatusCode, string(bodyBytes))
	}
}

func TestSessionsGeneratedSession(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1"}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result["session"] == "" {
		t.Fatalf("expected generated session, got empty")
	}
	if result["proxy_username"] == "" {
		t.Fatalf("expected proxy_username, got empty")
	}
}

func TestSessionsSessionKeyMatchesProxy(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8:1::1", "session": "abc123456", "location": "uk", "fallback": "yes"}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	key := routing.MakeSessionKey("john", "uk", "yes", "abc123456")
	storedIP, ok := s.sessionStore.Get(key)
	if !ok {
		t.Fatalf("session not found in store")
	}
	if storedIP.String() != "2001:db8:1::1" {
		t.Fatalf("expected 2001:db8:1::1, got %s", storedIP.String())
	}
}

func TestSessionsTTLAboveMax(t *testing.T) {
	s := newTestServerWithRouter()
	s.appCfg.MaxTimeout = 5
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1", "session": "abc123456", "ttl_minutes": 999}`)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	// TTL should be clamped to max (5 minutes = 300 seconds)
	if result["expires_at"] == "" {
		t.Fatalf("expected expires_at to be set")
	}
}

func TestSessionsSessionWithColon(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "2001:db8::1", "session": "abc:123"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSessionsBlockedIP(t *testing.T) {
	s := newTestServerWithRouter()
	resp := postSessions(t, s, `{"username": "john", "source_ip": "10.0.0.1", "session": "abc123456"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
