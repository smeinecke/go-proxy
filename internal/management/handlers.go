package management

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/routing"
)

const sessionIDChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateSessionID creates a cryptographically random session identifier.
func generateSessionID(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = sessionIDChars[int(b[i])%len(sessionIDChars)]
	}
	return string(b), nil
}

// buildProxyUsername builds the proxy username string from components.
func buildProxyUsername(username, session, location, fallback string) string {
	parts := []string{username}
	if session != "" {
		parts = append(parts, auth.ParamSession, session)
	}
	if location != "" {
		parts = append(parts, auth.ParamLocation, location)
	}
	if fallback != "" {
		parts = append(parts, auth.ParamFallback, fallback)
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "-" + parts[i]
	}
	return result
}

var (
	sessionRegex  = regexp.MustCompile(`^[a-zA-Z0-9]{6,24}$`)
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.]{1,64}$`)
)

func validFallback(v string) bool {
	return v == "" || v == "no" || v == "yes"
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": s.version,
		"commit":  s.commit,
		"date":    s.date,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.appCfg
	safeCfg := map[string]interface{}{
		"listen_address": cfg.ListenAddress,
		"listen_port":    cfg.ListenPort,
		"debug_mode":     cfg.DebugMode,
		"test_port":      cfg.TestPort,
		"network_type":   cfg.NetworkType,
		"max_timeout":    cfg.MaxTimeout,
		"idle_timeout":   cfg.IdleTimeout,
		"auth": map[string]interface{}{
			"type": cfg.Auth.Type,
		},
		"bind_prefixes":     cfg.BindPrefixes,
		"enable_fallback":   cfg.EnableFallback,
		"fallback_prefixes": cfg.FallbackPrefixes,
		"located_prefixes":  cfg.LocatedPrefixes,
		"replace_ips":       cfg.ReplaceIPs,
		"deleted_headers":   cfg.DeletedHeaders,
		"dns": map[string]interface{}{
			"type":    cfg.DNS.Type,
			"servers": cfg.DNS.Servers,
			"timeout": cfg.DNS.Timeout,
		},
		"blocked_cidrs": cfg.BlockedCIDRs,
		"management": map[string]interface{}{
			"enabled":        cfg.Management.Enabled,
			"listen_address": cfg.Management.ListenAddress,
			"port":           cfg.Management.Port,
			"allow_public":   cfg.Management.AllowPublic,
		},
	}
	writeJSON(w, http.StatusOK, safeCfg)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"stats": map[string]uint64{}})
		return
	}
	writeJSON(w, http.StatusOK, s.stats.Snapshot())
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}

	if s.sessionStore == nil || s.router == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "session store or router not configured")
		return
	}

	var req struct {
		Username  string `json:"username"`
		Session   string `json:"session"`
		SourceIP  string `json:"source_ip"`
		IP        string `json:"ip"` // deprecated alias for source_ip
		TTL       int    `json:"ttl_minutes"`
		Location  string `json:"location"`
		Fallback  string `json:"fallback"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Support both source_ip and the deprecated ip field
	if req.SourceIP == "" {
		req.SourceIP = req.IP
	}
	// Normalize bracketed IPv6
	req.SourceIP = strings.Trim(req.SourceIP, "[]")

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "username is required")
		return
	}
	if !usernameRegex.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "bad_request", "username must match ^[a-zA-Z0-9_.]{1,64}$")
		return
	}

	if !validFallback(req.Fallback) {
		writeError(w, http.StatusBadRequest, "bad_request", "fallback must be 'no' or 'yes'")
		return
	}

	// Pre-created sessions default to fallback=no for exact-IP semantics
	if req.Fallback == "" {
		req.Fallback = "no"
	}

	// Validate or generate session
	if req.Session == "" {
		generated, err := generateSessionID(12)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate session ID")
			return
		}
		req.Session = generated
	} else {
		if strings.Contains(req.Session, ":") {
			writeError(w, http.StatusBadRequest, "bad_request", "session must not contain ':'")
			return
		}
		if !sessionRegex.MatchString(req.Session) {
			writeError(w, http.StatusBadRequest, "bad_request", "session must be 6-24 alphanumeric characters")
			return
		}
	}

	// Validate source_ip
	if req.SourceIP == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "source_ip is required")
		return
	}

	ip, err := netip.ParseAddr(req.SourceIP)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid source_ip address")
		return
	}

	// Reject hostnames and malformed values (ParseAddr already rejects hostnames)
	// Reject IPs with ports
	if ip.IsUnspecified() {
		writeError(w, http.StatusBadRequest, "bad_request", "source_ip must not be unspecified")
		return
	}

	// Check blocked ranges
	if s.resolver != nil && s.resolver.IsBlocked(ip) {
		writeError(w, http.StatusForbidden, "forbidden", "source_ip is in a blocked range")
		return
	}

	// Validate IP against configured pools
	if ip.Is6() {
		if req.Location != "" {
			if !s.router.IsIPInLocatedPrefix(req.Location, ip) {
				writeError(w, http.StatusForbidden, "forbidden", "source_ip not in located_prefixes for location")
				return
			}
		} else if !s.router.IsIPInPool(ip) {
			writeError(w, http.StatusForbidden, "forbidden", "source_ip not in allowed bind_prefixes")
			return
		}
	} else if ip.Is4() {
		if !s.appCfg.EnableFallback {
			writeError(w, http.StatusForbidden, "forbidden", "IPv4 source_ip requires fallback to be enabled")
			return
		}
		if !s.router.IsIPInFallbackPrefix(ip) {
			writeError(w, http.StatusForbidden, "forbidden", "IPv4 source_ip must be within fallback_prefixes")
			return
		}
	}

	cacheKey := routing.MakeSessionKey(req.Username, req.Location, req.Fallback, req.Session)

	// Check for duplicate unless overwrite is requested
	if !req.Overwrite {
		if _, exists := s.sessionStore.Get(cacheKey); exists {
			writeError(w, http.StatusConflict, "conflict", "session already exists")
			return
		}
	}

	// Validate TTL
	maxTTL := time.Duration(s.appCfg.MaxTimeout) * time.Minute
	ttl := time.Duration(req.TTL) * time.Minute
	if ttl <= 0 {
		ttl = maxTTL
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}
	} else if maxTTL > 0 && ttl > maxTTL {
		writeError(w, http.StatusBadRequest, "bad_request", "ttl_minutes exceeds max_timeout")
		return
	}

	s.sessionStore.Set(cacheKey, ip, ttl)

	expiresAt := time.Now().UTC().Add(ttl)
	proxyUsername := buildProxyUsername(req.Username, req.Session, req.Location, req.Fallback)

	log.Info().
		Str("username", req.Username).
		Str("session", req.Session).
		Str("source_ip", ip.String()).
		Str("location", req.Location).
		Str("fallback", req.Fallback).
		Bool("overwrite", req.Overwrite).
		Dur("ttl", ttl).
		Msg("Management API: session created")

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"username":       req.Username,
		"session":        req.Session,
		"source_ip":      ip.String(),
		"location":       req.Location,
		"fallback":       req.Fallback,
		"proxy_username": proxyUsername,
		"expires_at":     expiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}
