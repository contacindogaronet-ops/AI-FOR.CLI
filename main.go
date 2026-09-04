package main

import (
	"os"

	cli "aicli/internal/cli"
	"aicli/internal/config"
	"aicli/internal/core"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Error().Err(err).Msg("Gagal memuat config.yaml")
		os.Exit(1)
	}

	pool := core.InitPool(cfg)
	memory := core.LoadMemory()

	// Masuk ke Interactive Menu
	cli.RunInteractiveMenu(cfg, pool, memory)
}
