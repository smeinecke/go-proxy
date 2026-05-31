package management

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"regexp"
	"time"

	"github.com/vlourme/go-proxy/internal/routing"
)

var sessionRegex = regexp.MustCompile(`^[a-zA-Z0-9]{6,24}$`)

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
		Username   string `json:"username"`
		Session    string `json:"session"`
		SourceIP   string `json:"source_ip"`
		IP         string `json:"ip"` // deprecated alias for source_ip
		TTL        int    `json:"ttl_minutes"`
		Location   string `json:"location"`
		Fallback   string `json:"fallback"`
		Overwrite  bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Support both source_ip and the deprecated ip field
	if req.SourceIP == "" {
		req.SourceIP = req.IP
	}

	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "username is required")
		return
	}
	if req.Session == "" || !sessionRegex.MatchString(req.Session) {
		writeError(w, http.StatusBadRequest, "bad_request", "session must be 6-24 alphanumeric characters")
		return
	}

	var ip netip.Addr
	var err error
	if req.SourceIP != "" {
		ip, err = netip.ParseAddr(req.SourceIP)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid source_ip address")
			return
		}

		// Validate IP against configured pools
		if ip.Is6() {
			if req.Location != "" {
				if !s.router.IsIPInLocatedPrefix(req.Location, ip) {
					writeError(w, http.StatusBadRequest, "bad_request", "source_ip not in located_prefixes for location")
					return
				}
			} else if !s.router.IsIPInPool(ip) {
				writeError(w, http.StatusBadRequest, "bad_request", "source_ip not in allowed bind_prefixes")
				return
			}
		} else if ip.Is4() {
			// IPv4 is only allowed from fallback_prefixes when fallback is enabled
			if !s.appCfg.EnableFallback || !s.router.IsIPInFallbackPrefix(ip) {
				writeError(w, http.StatusBadRequest, "bad_request", "IPv4 source_ip must be within fallback_prefixes and fallback must be enabled")
				return
			}
		}
	} else {
		// Auto-generate an IP from the pool
		route, err := s.router.Route(routing.RouteRequest{
			Username: req.Username,
			Location: req.Location,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate IP")
			return
		}
		ip = route.SourceIP
	}

	cacheKey := routing.MakeSessionKey(req.Username, req.Location, req.Fallback, req.Session)

	// Check for duplicate unless overwrite is requested
	if !req.Overwrite {
		if _, exists := s.sessionStore.Get(cacheKey); exists {
			writeError(w, http.StatusConflict, "conflict", "session already exists")
			return
		}
	}

	ttl := time.Duration(req.TTL) * time.Minute
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if ttl > time.Duration(s.appCfg.MaxTimeout)*time.Minute {
		ttl = time.Duration(s.appCfg.MaxTimeout) * time.Minute
	}

	s.sessionStore.Set(cacheKey, ip, ttl)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session":   req.Session,
		"username":  req.Username,
		"source_ip": ip.String(),
		"ttl_secs":  int(ttl.Seconds()),
		"mode":      "explicit",
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}
