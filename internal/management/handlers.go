package management

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"time"

	"github.com/vlourme/go-proxy/internal/routing"
)

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
		Session  string `json:"session"`
		IP       string `json:"ip"`
		TTL      int    `json:"ttl_minutes"`
		Location string `json:"location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.Session == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "session is required")
		return
	}

	var ip netip.Addr
	var err error
	if req.IP != "" {
		ip, err = netip.ParseAddr(req.IP)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid IP address")
			return
		}
	} else {
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

	ttl := time.Duration(req.TTL) * time.Minute
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if ttl > time.Duration(s.appCfg.MaxTimeout)*time.Minute {
		ttl = time.Duration(s.appCfg.MaxTimeout) * time.Minute
	}

	cacheKey := routing.SessionKey(req.Username + ":" + req.Location + "::" + req.Session)
	s.sessionStore.Set(cacheKey, ip, ttl)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"session":   req.Session,
		"username":  req.Username,
		"ip":        ip.String(),
		"ttl_secs":  int(ttl.Seconds()),
		"mode":      "explicit",
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}
