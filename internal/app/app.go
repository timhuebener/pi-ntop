package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"pi-ntop/internal/config"
	appdb "pi-ntop/internal/db"
	"pi-ntop/internal/httpserver"
	"pi-ntop/internal/monitor"
)

type App struct {
	config  config.Config
	db      *sql.DB
	monitor *monitor.Service
	server  *http.Server
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	database, err := appdb.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := appdb.RunMigrations(database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	monitorService := monitor.New(database, cfg)
	server := httpserver.New(cfg, database, monitorService)

	return &App{
		config:  cfg,
		db:      database,
		monitor: monitorService,
		server:  server,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.monitor.Run(ctx)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
		defer cancel()
		_ = a.server.Shutdown(shutdownCtx)
	}()

	err := a.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (a *App) Close() error {
	if a.db == nil {
		return nil
	}
	return a.db.Close()
}
