package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/core"
	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/baseline/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := os.Getenv("SNT_BASELINE_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("SNT_BASELINE_DATABASE_URL is required")
		os.Exit(1)
	}
	credentials, err := core.ParseCredentials(os.Getenv("SNT_BASELINE_API_KEYS"))
	if err != nil {
		logger.Error("invalid API key configuration", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("invalid database configuration", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	pingContext, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingContext); err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	address := os.Getenv("SNT_BASELINE_LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}
	server := &http.Server{
		Addr: address, Handler: httpapi.New(core.NewStore(pool, credentials), logger),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	stopContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-stopContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Error("server shutdown failed", "error", err)
		}
	}()
	logger.Info("baseline API listening", "address", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
