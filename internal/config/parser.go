package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/utils"
)

type AuthType string

const (
	AuthTypeNone        AuthType = "none"
	AuthTypeCredentials AuthType = "credentials"
	AuthTypeRedis       AuthType = "redis"
)

// DNSConfig holds DNS resolver settings.
type DNSConfig struct {
	Type    string   `yaml:"type"`    // "system" or "custom"
	Servers []string `yaml:"servers"` // list of DNS servers for custom type (e.g., "1.1.1.1:53")
	Timeout int      `yaml:"timeout"` // timeout in seconds
}

// Config is the configuration for the proxy.
type Config struct {
	// ListenAddress is the address to listen on.
	ListenAddress string `yaml:"listen_address"`
	// ListenPort is the port to listen on.
	ListenPort uint16 `yaml:"listen_port"`
	// DebugMode is whether to enable debug mode.
	DebugMode bool `yaml:"debug_mode"`
	// TestPort is the port to test the proxy. -1 disables.
	TestPort int `yaml:"test_port"`
	// NetworkType is the network type for listeners: tcp, tcp4, tcp6.
	NetworkType string `yaml:"network_type"`
	// MaxTimeout is the maximum timeout for a session in minutes.
	MaxTimeout int `yaml:"max_timeout"`
	// IdleTimeout is the idle timeout for a tunnel in seconds.
	IdleTimeout int `yaml:"idle_timeout"`
	// Auth is the authentication configuration.
	Auth struct {
		Type        AuthType `yaml:"type"`
		Credentials struct {
			Username string `yaml:"username"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
		Redis struct {
			DSN string `yaml:"dsn"`
		} `yaml:"redis"`
	} `yaml:"auth"`
	// BindPrefixes is the list of prefixes to bind to.
	BindPrefixes []string `yaml:"bind_prefixes"`
	// EnableFallback is whether to enable the fallback prefix.
	EnableFallback bool `yaml:"enable_fallback"`
	// FallbackPrefixes is the list of IPv4 prefixes to bind if the target does not support IPv6.
	FallbackPrefixes []string `yaml:"fallback_prefixes"`
	// LocatedPrefixes is the list of prefixes to bind to for each location.
	LocatedPrefixes map[string][]string `yaml:"located_prefixes"`
	// ReplaceIPs is the list of IPs to replace with the override.
	ReplaceIPs map[string]string `yaml:"replace_ips"`
	// DeletedHeaders is the list of headers to delete.
	DeletedHeaders []string `yaml:"deleted_headers"`
	// DNS configuration.
	DNS DNSConfig `yaml:"dns"`
	// BlockedCIDRs are extra CIDRs to block in addition to the default private ranges.
	BlockedCIDRs []string `yaml:"blocked_cidrs"`
}

var config *Config
var bindPrefixes = []net.IPNet{}
var fallbackPrefixes = []net.IPNet{}
var locatedPrefixes = map[string][]net.IPNet{}
var replaceIPs = map[*net.IPNet]string{}
var blockedNets = []net.IPNet{}

var defaultBlockedNets = func() []net.IPNet {
	defaults := []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "100.64.0.0/10", "0.0.0.0/8",
		"::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	var nets []net.IPNet
	for _, cidr := range defaults {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		nets = append(nets, *ipnet)
	}
	return nets
}()

func load() *Config {
	path := flag.String("config", "config.yaml", "The path to the config file")
	flag.Parse()

	yamlFile, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal().Err(err).Msg("Error reading config file")
	}

	var cfg Config
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing config file")
	}

	if err := validate(&cfg); err != nil {
		log.Fatal().Err(err).Msg("Config validation failed")
	}

	for _, prefix := range cfg.BindPrefixes {
		_, ipnet, _ := net.ParseCIDR(prefix)
		bindPrefixes = append(bindPrefixes, *ipnet)
	}

	for _, prefix := range cfg.FallbackPrefixes {
		_, ipnet, _ := net.ParseCIDR(prefix)
		fallbackPrefixes = append(fallbackPrefixes, *ipnet)
	}

	for location, prefixes := range cfg.LocatedPrefixes {
		for _, prefix := range prefixes {
			_, ipnet, _ := net.ParseCIDR(prefix)
			locatedPrefixes[location] = append(locatedPrefixes[location], *ipnet)
		}
	}

	for cidr, ip := range cfg.ReplaceIPs {
		_, ipnet, _ := net.ParseCIDR(cidr)
		replaceIPs[ipnet] = ip
	}

	for _, cidr := range cfg.BlockedCIDRs {
		_, ipnet, _ := net.ParseCIDR(cidr)
		blockedNets = append(blockedNets, *ipnet)
	}

	return &cfg
}

func validate(cfg *Config) error {
	if cfg.NetworkType == "" {
		cfg.NetworkType = "tcp"
	}
	if cfg.NetworkType != "tcp" && cfg.NetworkType != "tcp4" && cfg.NetworkType != "tcp6" {
		return fmt.Errorf("invalid network_type: %q (must be tcp, tcp4, or tcp6)", cfg.NetworkType)
	}

	if cfg.ListenPort == 0 {
		return errors.New("listen_port must be set")
	}

	if cfg.MaxTimeout <= 0 {
		cfg.MaxTimeout = 30
	}

	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30
	}

	if cfg.ListenAddress != "" && net.ParseIP(cfg.ListenAddress) == nil {
		return fmt.Errorf("invalid listen_address: %q", cfg.ListenAddress)
	}

	if len(cfg.BindPrefixes) == 0 {
		return errors.New("bind_prefixes must contain at least one prefix")
	}

	for _, prefix := range cfg.BindPrefixes {
		_, ipnet, err := net.ParseCIDR(prefix)
		if err != nil {
			return fmt.Errorf("invalid bind prefix %q: %w", prefix, err)
		}
		if !isByteAligned(*ipnet) {
			return fmt.Errorf("bind prefix %q is not byte-aligned (must be multiple of 8, e.g., /48, /56, /64)", prefix)
		}
	}

	for _, prefix := range cfg.FallbackPrefixes {
		_, ipnet, err := net.ParseCIDR(prefix)
		if err != nil {
			return fmt.Errorf("invalid fallback prefix %q: %w", prefix, err)
		}
		if ipnet.IP.To4() == nil {
			return fmt.Errorf("fallback prefix %q must be IPv4", prefix)
		}
		if !isByteAligned(*ipnet) {
			return fmt.Errorf("fallback prefix %q is not byte-aligned (must be multiple of 8)", prefix)
		}
	}

	if cfg.EnableFallback && len(cfg.FallbackPrefixes) == 0 {
		return errors.New("fallback_prefixes must contain at least one prefix when enable_fallback is true")
	}

	for location, prefixes := range cfg.LocatedPrefixes {
		if len(prefixes) == 0 {
			return fmt.Errorf("located prefix list for %q is empty", location)
		}
		for _, prefix := range prefixes {
			_, ipnet, err := net.ParseCIDR(prefix)
			if err != nil {
				return fmt.Errorf("invalid located prefix %q for %q: %w", prefix, location, err)
			}
			if !isByteAligned(*ipnet) {
				return fmt.Errorf("located prefix %q for %q is not byte-aligned", prefix, location)
			}
		}
	}

	for cidr, ip := range cfg.ReplaceIPs {
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("invalid replace_ips CIDR %q: %w", cidr, err)
		}
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return fmt.Errorf("invalid replace_ips IP %q", ip)
		}
		for _, n := range defaultBlockedNets {
			if n.Contains(parsedIP) {
				return fmt.Errorf("replace_ips IP %q is in a default blocked range (%s)", ip, n.String())
			}
		}
		for _, blockedCIDR := range cfg.BlockedCIDRs {
			_, ipnet, _ := net.ParseCIDR(blockedCIDR)
			if ipnet != nil && ipnet.Contains(parsedIP) {
				return fmt.Errorf("replace_ips IP %q is in blocked_cidrs %s", ip, blockedCIDR)
			}
		}
	}

	switch cfg.Auth.Type {
	case AuthTypeNone, AuthTypeCredentials, AuthTypeRedis:
		// valid
	case "":
		return errors.New("auth.type must be explicitly set (none, credentials, or redis)")
	default:
		return fmt.Errorf("invalid auth.type: %q (must be none, credentials, or redis)", cfg.Auth.Type)
	}

	if cfg.Auth.Type == AuthTypeCredentials {
		if cfg.Auth.Credentials.Username == "" || cfg.Auth.Credentials.Password == "" {
			return errors.New("auth credentials username and password must be set")
		}
	}

	if cfg.Auth.Type == AuthTypeRedis {
		if cfg.Auth.Redis.DSN == "" {
			return errors.New("auth redis dsn must be set when auth.type is redis")
		}
	}

	if cfg.DNS.Type != "" && cfg.DNS.Type != "system" && cfg.DNS.Type != "custom" {
		return fmt.Errorf("invalid dns.type: %q (must be system or custom)", cfg.DNS.Type)
	}
	if cfg.DNS.Type == "custom" && len(cfg.DNS.Servers) == 0 {
		return errors.New("dns.servers must be set when dns.type is custom")
	}
	if cfg.DNS.Timeout <= 0 {
		cfg.DNS.Timeout = 5
	}

	for _, cidr := range cfg.BlockedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid blocked_cidrs %q: %w", cidr, err)
		}
	}

	return nil
}

func isByteAligned(ipnet net.IPNet) bool {
	ones, _ := ipnet.Mask.Size()
	return ones%8 == 0
}

// Get returns the parsed config
func Get() *Config {
	if config == nil {
		config = load()
	}

	return config
}

// GetBindPrefixes returns the bind prefixes
func GetBindPrefixes() []net.IPNet {
	return bindPrefixes
}

// GetAnyBindPrefix returns a random bind prefix
func GetAnyBindPrefix() net.IPNet {
	return bindPrefixes[utils.RandomInt(len(bindPrefixes))]
}

// GetFallbackPrefixes returns the fallback prefixes
func GetFallbackPrefixes() []net.IPNet {
	return fallbackPrefixes
}

// GetAnyFallbackPrefix returns a random fallback prefix
func GetAnyFallbackPrefix() net.IPNet {
	return fallbackPrefixes[utils.RandomInt(len(fallbackPrefixes))]
}

// GetLocatedPrefixes returns the located prefixes
func GetLocatedPrefixes() map[string][]net.IPNet {
	return locatedPrefixes
}

// GetReplaceIPs returns the replace IPs
func GetReplaceIPs() map[*net.IPNet]string {
	return replaceIPs
}

// GetBlockedNets returns the user-defined blocked CIDRs
func GetBlockedNets() []net.IPNet {
	return blockedNets
}

// DNSTimeout returns the configured DNS timeout.
func DNSTimeout() time.Duration {
	return time.Duration(Get().DNS.Timeout) * time.Second
}
