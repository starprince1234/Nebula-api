package main

import (
	"context"
	"github.com/starprince1234/Nebula-api/internal/config"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db"
	"github.com/starprince1234/Nebula-api/internal/usage"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.LoadDatabaseURL()
	if err != nil {
		slog.Error("load maintenance configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client, sqlDB, err := db.Open(ctx, cfg)
	if err != nil {
		slog.Error("open maintenance database", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	store := usage.NewStore(sqlDB)
	if err := store.RunMaintenance(ctx, true); err != nil {
		slog.Error("initial maintenance cycle failed", "error", err)
	}
	ticker := time.NewTicker(time.Minute)
	hourly := time.NewTicker(time.Hour)
	defer ticker.Stop()
	defer hourly.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.RunMaintenance(ctx, false); err != nil {
				slog.Error("maintenance cycle failed", "error", err)
			}
		case <-hourly.C:
			if err := store.RunMaintenance(ctx, true); err != nil {
				slog.Error("hourly maintenance cycle failed", "error", err)
			}
		}
	}
}
