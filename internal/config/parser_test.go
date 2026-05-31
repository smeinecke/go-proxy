package config

import (
	"testing"
)

func authNone() struct {
	Type        AuthType `yaml:"type"`
	Credentials struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"credentials"`
	Redis struct {
		DSN string `yaml:"dsn"`
	} `yaml:"redis"`
} {
	return struct {
		Type        AuthType `yaml:"type"`
		Credentials struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
		Redis struct {
			DSN string `yaml:"dsn"`
		} `yaml:"redis"`
	}{Type: AuthTypeNone}
}

func TestValidateBasic(t *testing.T) {
	cfg := &Config{
		ListenPort:     8080,
		NetworkType:    "tcp6",
		BindPrefixes:   []string{"2001:db8::/48"},
		MaxTimeout:     30,
		EnableFallback: false,
		Auth:           authNone(),
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.NetworkType != "tcp6" {
		t.Fatalf("expected network_type tcp6, got %s", cfg.NetworkType)
	}
}

func TestValidateDefaults(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		Auth:         authNone(),
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.NetworkType != "tcp" {
		t.Fatalf("expected default network_type tcp, got %s", cfg.NetworkType)
	}
	if cfg.MaxTimeout != 30 {
		t.Fatalf("expected default max_timeout 30, got %d", cfg.MaxTimeout)
	}
	if cfg.IdleTimeout != 30 {
		t.Fatalf("expected default idle_timeout 30, got %d", cfg.IdleTimeout)
	}
}

func TestValidateInvalidNetworkType(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		NetworkType:  "udp",
		BindPrefixes: []string{"2001:db8::/48"},
		Auth:         authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for invalid network_type")
	}
}

func TestValidateMissingBindPrefix(t *testing.T) {
	cfg := &Config{
		ListenPort: 8080,
		Auth:       authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for missing bind_prefixes")
	}
}

func TestValidateNonByteAlignedPrefix(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/52"},
		Auth:         authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for non-byte-aligned prefix")
	}
}

func TestValidateFallbackIPv4Only(t *testing.T) {
	cfg := &Config{
		ListenPort:       8080,
		BindPrefixes:     []string{"2001:db8::/48"},
		EnableFallback:   true,
		FallbackPrefixes: []string{"2001:db8::/48"},
		Auth:             authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for IPv6 fallback prefix")
	}
}

func TestValidateFallbackMissing(t *testing.T) {
	cfg := &Config{
		ListenPort:     8080,
		BindPrefixes:   []string{"2001:db8::/48"},
		EnableFallback: true,
		Auth:           authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for missing fallback_prefixes")
	}
}

func TestValidateEmptyLocatedPrefix(t *testing.T) {
	cfg := &Config{
		ListenPort:      8080,
		BindPrefixes:    []string{"2001:db8::/48"},
		LocatedPrefixes: map[string][]string{"us": {}},
		Auth:            authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for empty located prefix list")
	}
}

func TestValidateInvalidReplaceIP(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		ReplaceIPs:   map[string]string{"not-a-cidr": "1.2.3.4"},
		Auth:         authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for invalid replace_ips CIDR")
	}
}

func TestValidateCredentialsMissing(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		Auth: struct {
			Type        AuthType `yaml:"type"`
			Credentials struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"credentials"`
			Redis struct {
				DSN string `yaml:"dsn"`
			} `yaml:"redis"`
		}{Type: AuthTypeCredentials},
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for missing credentials")
	}
}

func TestValidateInvalidListenAddress(t *testing.T) {
	cfg := &Config{
		ListenPort:    8080,
		BindPrefixes:  []string{"2001:db8::/48"},
		ListenAddress: "not-an-ip",
		Auth:          authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for invalid listen_address")
	}
}

func TestValidateMissingAuthType(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for missing auth.type")
	}
}

func TestValidateInvalidAuthType(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		Auth: struct {
			Type        AuthType `yaml:"type"`
			Credentials struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"credentials"`
			Redis struct {
				DSN string `yaml:"dsn"`
			} `yaml:"redis"`
		}{Type: "invalid"},
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for invalid auth.type")
	}
}

func TestValidateRedisDSNMissing(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		Auth: struct {
			Type        AuthType `yaml:"type"`
			Credentials struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"credentials"`
			Redis struct {
				DSN string `yaml:"dsn"`
			} `yaml:"redis"`
		}{Type: AuthTypeRedis},
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for missing redis dsn")
	}
}

func TestValidateFallbackAlwaysValidated(t *testing.T) {
	cfg := &Config{
		ListenPort:       8080,
		BindPrefixes:     []string{"2001:db8::/48"},
		EnableFallback:   false,
		FallbackPrefixes: []string{"bad-prefix"},
		Auth:             authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for invalid fallback_prefix even when enable_fallback is false")
	}
}

func TestValidateCustomDNSMissingServers(t *testing.T) {
	cfg := &Config{
		ListenPort:   8080,
		BindPrefixes: []string{"2001:db8::/48"},
		DNS:          DNSConfig{Type: "custom"},
		Auth:         authNone(),
	}
	if err := validate(cfg); err == nil {
		t.Fatalf("expected error for custom DNS without servers")
	}
}
