package management

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/config"
)

// Server is the management API HTTP server.
type Server struct {
	appCfg   *config.Config
	http     *http.Server
	listener net.Listener
	version  string
	commit   string
	date     string
}

// New creates a new management server.
func New(appCfg *config.Config, version, commit, date string) *Server {
	return &Server{
		appCfg:  appCfg,
		version: version,
		commit:  commit,
		date:    date,
	}
}

func (s *Server) router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.authMiddleware(requireGET(s.handleHealthz)))
	mux.HandleFunc("/api/v1/status", s.authMiddleware(requireGET(s.handleStatus)))
	mux.HandleFunc("/api/v1/config", s.authMiddleware(requireGET(s.handleConfig)))
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// Start binds and starts the management server in a background goroutine.
// If binding fails, an error is returned synchronously.
func (s *Server) Start() error {
	addr := net.JoinHostPort(s.appCfg.Management.ListenAddress, strconv.Itoa(s.appCfg.Management.Port))
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind management server on %s: %w", addr, err)
	}
	s.listener = listener

	log.Info().Str("address", listener.Addr().String()).Msg("Starting management server")
	go func() {
		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Management server error")
		}
	}()
	return nil
}

// Addr returns the actual listening address of the management server.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop gracefully shuts down the management server.
func (s *Server) Stop(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}
