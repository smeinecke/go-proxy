package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"time"

	"github.com/libp2p/go-reuseport"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/auth"
	"github.com/vlourme/go-proxy/internal/config"
	"github.com/vlourme/go-proxy/internal/handlers"
	"github.com/vlourme/go-proxy/internal/management"
	"github.com/vlourme/go-proxy/internal/routing"
	"github.com/vlourme/go-proxy/internal/stats"
	"github.com/vlourme/go-proxy/internal/sys"
)

// App holds all runtime dependencies and services.
type App struct {
	Config        *config.Config
	Authenticator auth.Authenticator
	Router        *routing.Router
	Resolver      *routing.Resolver
	SessionStore  routing.SessionStore
	Stats         *stats.Container
	Management    *management.Server

	listeners []net.Listener
	version   string
	commit    string
	date      string
}

// New creates an App from the loaded config and build version info.
func New(cfg *config.Config, version, commit, date string) (*App, error) {
	bindPrefixes, err := parsePrefixes(cfg.BindPrefixes)
	if err != nil {
		return nil, fmt.Errorf("bind prefixes: %w", err)
	}

	fallbackPrefixes, err := parsePrefixes(cfg.FallbackPrefixes)
	if err != nil {
		return nil, fmt.Errorf("fallback prefixes: %w", err)
	}

	locatedPrefixes := make(map[string][]netip.Prefix)
	for loc, prefixes := range cfg.LocatedPrefixes {
		pps, err := parsePrefixes(prefixes)
		if err != nil {
			return nil, fmt.Errorf("located prefixes for %s: %w", loc, err)
		}
		locatedPrefixes[loc] = pps
	}

	blockedPrefixes, err := parsePrefixes(cfg.BlockedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("blocked cidrs: %w", err)
	}
	// Add default blocked ranges
	defaultBlocked := []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "100.64.0.0/10", "0.0.0.0/8",
		"::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	for _, cidr := range defaultBlocked {
		p, err := netip.ParsePrefix(cidr)
		if err == nil {
			blockedPrefixes = append(blockedPrefixes, p)
		}
	}

	var replaces []routing.ReplacementRule
	for cidr, ipStr := range cfg.ReplaceIPs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("replace_ips prefix %q: %w", cidr, err)
		}
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			return nil, fmt.Errorf("replace_ips IP %q: %w", ipStr, err)
		}
		replaces = append(replaces, routing.ReplacementRule{Prefix: prefix, IP: ip})
	}

	sessionStore := routing.NewSessionStore(1024 * 1024)

	router := routing.NewRouter(
		sessionStore,
		bindPrefixes,
		fallbackPrefixes,
		locatedPrefixes,
		time.Duration(cfg.MaxTimeout)*time.Minute,
		cfg.EnableFallback,
	)

	resolver := routing.NewResolver(
		cfg.DNS.Type,
		cfg.DNS.Servers,
		time.Duration(cfg.DNS.Timeout)*time.Second,
		cfg.DebugMode,
		blockedPrefixes,
		replaces,
	)

	authenticator, err := auth.NewAuthenticator(cfg)
	if err != nil {
		return nil, fmt.Errorf("auth setup: %w", err)
	}

	app := &App{
		Config:        cfg,
		Authenticator: authenticator,
		Router:        router,
		Resolver:      resolver,
		SessionStore:  sessionStore,
		version:       version,
		commit:        commit,
		date:          date,
	}

	if *cfg.EnableStats {
		app.Stats = stats.NewContainer(runtime.NumCPU())
	}

	if cfg.Management.Enabled {
		app.Management = management.New(cfg, version, commit, date)
		app.Management.SetStats(app.Stats)
		app.Management.SetSessionStore(app.SessionStore)
		app.Management.SetRouter(app.Router)
		app.Management.SetResolver(app.Resolver)
	}

	return app, nil
}

// Run starts all services and blocks until the context is cancelled.
func (a *App) Run(ctx context.Context) error {
	if a.Config.DebugMode {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	sys.TuneSysctl()
	for _, prefix := range a.Config.BindPrefixes {
		if err := sys.AddRoute(prefix); err != nil {
			if err.Error() == "file exists" {
				log.Info().Str("prefix", prefix).Msg("Route already exists")
			} else {
				log.Error().Err(err).Str("prefix", prefix).Msg("Failed to add route")
			}
		}
	}

	if a.Management != nil {
		if err := a.Management.Start(); err != nil {
			return fmt.Errorf("start management server: %w", err)
		}
		defer a.Management.Stop(context.Background())
	}

	if a.Config.TestPort > 0 {
		go a.runTestServer()
	}

	addr := net.TCPAddr{
		IP:   net.ParseIP(a.Config.ListenAddress),
		Port: int(a.Config.ListenPort),
	}

	log.Info().Int("count", runtime.NumCPU()).Str("address", addr.String()).Msg("Starting listeners")

	handler := &handlers.ProxyHandler{
		Authenticator: a.Authenticator,
		Router:        a.Router,
		Resolver:      a.Resolver,
		Stats:         a.Stats,
		Config:        a.Config,
	}

	for range runtime.NumCPU() {
		listener, err := reuseport.Listen(a.Config.NetworkType, addr.String())
		if err != nil {
			return fmt.Errorf("create listener: %w", err)
		}
		a.listeners = append(a.listeners, listener)
	}

	for idx := range runtime.NumCPU() {
		go func(idx int) {
			listener := a.listeners[idx]
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				conn, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Error().Err(err).Msg("Failed to accept connection")
					continue
				}

				go handler.HandleConnection(idx, conn)
			}
		}(idx)
	}

	<-ctx.Done()

	for _, l := range a.listeners {
		l.Close()
	}

	if ctx.Err() == context.Canceled {
		return nil
	}
	return ctx.Err()
}

func (a *App) runTestServer() {
	server := &http.Server{
		Addr: fmt.Sprintf("[::]:%d", a.Config.TestPort),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Hello, World!"))
		}),
	}
	if err := server.ListenAndServe(); err != nil {
		log.Error().Err(err).Msg("Failed to start test server")
	}
}

func parsePrefixes(strs []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(strs))
	for _, s := range strs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", s, err)
		}
		out = append(out, p)
	}
	return out, nil
}
