package db

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
)

const DriverName = "pgx"

func Open(ctx context.Context, databaseURL string) (*ent.Client, *sql.DB, error) {
	sqlDB, err := sql.Open(DriverName, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	driver := entsql.OpenDB(dialect.Postgres, sqlDB)
	return ent.NewClient(ent.Driver(driver)), sqlDB, nil
}
