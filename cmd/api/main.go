package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	"github.com/kumparan/article-service/internal/config"
	httphandler "github.com/kumparan/article-service/internal/handler/http"
	"github.com/kumparan/article-service/internal/platform/logger"
	"github.com/kumparan/article-service/internal/platform/postgres"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:")
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	log := logger.New(cfg.AppEnv, os.Getenv("LOG_LEVEL"))
	log.Info("article-service api starting", "env", cfg.AppEnv, "addr", cfg.HTTP.Addr)

	db, err := postgres.Open(context.Background(), cfg.DB)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Wire repositories and application service.
	articleRepo := pgrepo.NewArticleRepository(db)
	authorRepo := pgrepo.NewAuthorRepository(db)
	outboxRepo := pgrepo.NewOutboxRepository(db, 5*time.Minute, cfg.Outbox.MaxAttempts)
	articleSvc := appsvc.New(db, articleRepo, authorRepo, outboxRepo)

	router := httphandler.NewRouter(db, articleSvc)

	srv := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("shutdown complete")
}
