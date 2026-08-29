package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/starprince1234/Nebula-api/internal/config"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		slog.Error("load migration configuration", "error", err)
		os.Exit(1)
	}
	client, sqlDB, err := db.Open(ctx, databaseURL)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := db.Migrate(ctx, sqlDB); err != nil {
		slog.Error("apply database migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database schema is up to date")
}
