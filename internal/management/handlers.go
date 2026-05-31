package management

import (
	"net/http"
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

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}
