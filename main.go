package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/vlourme/go-proxy/internal/app"
	"github.com/vlourme/go-proxy/internal/config"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cfg := config.Get()

	application, err := app.New(cfg, version, commit, date)
	if err != nil {
		log.Fatal().Err(err).Msg("startup failed")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		if err != context.Canceled {
			log.Fatal().Err(err).Msg("app stopped")
		}
	}
}
