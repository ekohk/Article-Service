package main

import (
	"fmt"
	"os"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:")
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	log := logger.New(cfg.AppEnv, os.Getenv("LOG_LEVEL"))
	log.Info("article-service worker starting", "env", cfg.AppEnv)

	// Outbox worker wiring is implemented in IMPL-12.
}
