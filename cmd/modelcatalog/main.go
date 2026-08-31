package main

import (
	"context"
	"github.com/starprince1234/Nebula-api/internal/catalog"
	"github.com/starprince1234/Nebula-api/internal/config"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		slog.Error("load catalog database configuration", "error", err)
		os.Exit(1)
	}
	apiKey := strings.TrimSpace(os.Getenv("MATRIX_APIKEY"))
	if apiKey == "" {
		slog.Error("MATRIX_APIKEY is required")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		slog.Error("open catalog database", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	syncer := &catalog.Syncer{DB: sqlDB, APIKey: apiKey}
	run := func() {
		if err := syncer.Sync(ctx); err != nil {
			slog.Error("matrix catalog sync failed", "error", err)
		} else {
			slog.Info("matrix catalog sync completed")
		}
	}
	run()
	location, _ := time.LoadLocation("Asia/Shanghai")
	for {
		now := time.Now().In(location)
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			run()
		}
	}
}
