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
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.RecoverAndClean(ctx); err != nil {
				slog.Error("maintenance cycle failed", "error", err)
			}
		}
	}
}
