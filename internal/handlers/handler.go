package handlers

import (
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/config"
	"github.com/vlourme/go-proxy/internal/routing"
	"github.com/vlourme/go-proxy/internal/stats"
)

// ProxyHandler holds all dependencies needed to handle proxy connections.
type ProxyHandler struct {
	Authenticator auth.Authenticator
	Router        *routing.Router
	Resolver      *routing.Resolver
	Stats         *stats.Stats
	Config        *config.Config
}
