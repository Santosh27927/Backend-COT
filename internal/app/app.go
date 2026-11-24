package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
	"github.com/Santosh27927/Backend-COT/internal/config"
	"github.com/Santosh27927/Backend-COT/internal/database"
	"github.com/Santosh27927/Backend-COT/internal/httpapi"
	"github.com/Santosh27927/Backend-COT/internal/scenarios"
	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/migrations"
)

func Run(service string) int {
	settings, err := config.Load(service, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 1
	}
	var level slog.Level
	_ = level.UnmarshalText([]byte(settings.LogLevel))
	baseLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("version", httpapi.Version)
	logger := baseLogger.With("service", service)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, settings.Database)
	if err != nil {
		logger.Error("database configuration rejected", "code", "DATABASE_CONFIG_INVALID")
		return 1
	}
	defer pool.Close()
	known, err := migrations.Embedded()
	if err != nil {
		logger.Error("embedded migrations invalid", "code", "MIGRATIONS_INVALID")
		return 1
	}
	ready := func(ctx context.Context) (bool, string) { return migrations.Ready(ctx, pool, known) }
	auth := authentication.NewDevelopment(settings.DevelopmentToken, settings.DevelopmentPlayerID)
	var routes []func(*http.ServeMux)
	if service == "game-service" {
		catalog, catalogErr := scenarios.Catalog()
		if catalogErr != nil {
			logger.Error("scripted catalog invalid", "code", "CATALOG_INVALID")
			return 1
		}
		store, err := sessions.NewCatalog(pool, catalog)
		if err != nil {
			logger.Error("scenario configuration rejected", "code", "SCENARIO_INVALID")
			return 1
		}
		routes = append(routes, httpapi.LearningRoutes(auth, store, logger))
		workerCtx, cancelWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() { defer close(workerDone); store.RunWorker(workerCtx, logger) }()
		defer func() { cancelWorker(); <-workerDone }()
	}
	server := &http.Server{Addr: settings.Address, Handler: httpapi.Handler(service, baseLogger, auth, ready, routes...), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	logger.Info("service starting", "address", settings.Address, "auth_mode", "dev-token")
	select {
	case err = <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listener failed", "code", "HTTP_LISTEN_FAILED")
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
		defer cancel()
		if err = server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			logger.Error("graceful shutdown timed out", "code", "HTTP_SHUTDOWN_TIMEOUT")
			return 1
		}
	}
	logger.Info("service stopped")
	return 0
}
