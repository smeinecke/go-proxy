package auth

import (
	"encoding/base64"
	"testing"

	"github.com/vlourme/go-proxy/internal/http"
)

func TestGetCredentialsValid(t *testing.T) {
	req := &http.Request{Header: make(map[string][]byte)}
	req.SetHeader("Proxy-Authorization", []byte("Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass"))))
	user, pass, params := GetCredentials(req)
	if user != "user" || pass != "pass" {
		t.Fatalf("expected user:pass, got %s:%s", user, pass)
	}
	if len(params) != 0 {
		t.Fatalf("expected no params, got %v", params)
	}
}

func TestGetCredentialsLowercaseScheme(t *testing.T) {
	req := &http.Request{Header: make(map[string][]byte)}
	req.SetHeader("proxy-authorization", []byte("basic "+base64.StdEncoding.EncodeToString([]byte("user:pass"))))
	user, pass, _ := GetCredentials(req)
	if user != "user" || pass != "pass" {
		t.Fatalf("expected user:pass, got %s:%s", user, pass)
	}
}

func TestGetCredentialsMalformedBase64(t *testing.T) {
	req := &http.Request{Header: make(map[string][]byte)}
	req.SetHeader("Proxy-Authorization", []byte("Basic not-valid-base64!!!"))
	user, pass, _ := GetCredentials(req)
	if user != "" || pass != "" {
		t.Fatalf("expected empty credentials for malformed base64, got %s:%s", user, pass)
	}
}

func TestGetCredentialsMissingColon(t *testing.T) {
	req := &http.Request{Header: make(map[string][]byte)}
	req.SetHeader("Proxy-Authorization", []byte("Basic "+base64.StdEncoding.EncodeToString([]byte("nocolon"))))
	user, pass, _ := GetCredentials(req)
	if user != "" || pass != "" {
		t.Fatalf("expected empty credentials for missing colon, got %s:%s", user, pass)
	}
}

func TestGetCredentialsEmptyUsername(t *testing.T) {
	req := &http.Request{Header: make(map[string][]byte)}
	req.SetHeader("Proxy-Authorization", []byte("Basic "+base64.StdEncoding.EncodeToString([]byte(":pass"))))
	user, pass, _ := GetCredentials(req)
	if user != "" || pass != "pass" {
		t.Fatalf("expected empty user with pass, got %s:%s", user, pass)
	}
}

func TestVerifySession(t *testing.T) {
	tests := []struct {
		session string
		valid   bool
	}{
		{"abcdef1234", true},
		{"ABCDEF1234", true},
		{"abc123", true},
		{"abc123456789012345678901", true},
		{"abc", false},
		{"abc1234567890123456789012", false},
		{"abc-def", false},
		{"abc_123", false},
		{"abc 123", false},
		{"", false},
		{"000000", true},
	}

	for _, tc := range tests {
		result := make(map[string]string)
		result[ParamSession] = tc.session
		got := VerifySession(result)
		if got != tc.valid {
			t.Errorf("VerifySession(%q) = %v, want %v", tc.session, got, tc.valid)
		}
	}
}

func TestGetParams(t *testing.T) {
	params := GetParams("session-abc123-country-uk")
	if params[ParamSession] != "abc123" {
		t.Fatalf("expected session abc123, got %s", params[ParamSession])
	}
	if params[ParamLocation] != "uk" {
		t.Fatalf("expected country uk, got %s", params[ParamLocation])
	}
}

func TestSplitParams(t *testing.T) {
	user, params := SplitParams("john-session-abc123")
	if user != "john" || params != "session-abc123" {
		t.Fatalf("expected john / session-abc123, got %s / %s", user, params)
	}
}
