package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationLockID int64 = 7_823_409_117

func Migrate(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		_, _ = database.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()
	if _, err := database.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version varchar(128) PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", name).Scan(&exists); err != nil {
			return fmt.Errorf("query migration %s: %w", name, err)
		}
		if exists {
			continue
		}
		if name == "0001_v06_baseline.sql" {
			empty, err := databaseIsEmpty(ctx, database)
			if err != nil {
				return err
			}
			if !empty {
				if err := validateV06Fingerprint(ctx, database); err != nil {
					return err
				}
				if _, err := database.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", name); err != nil {
					return fmt.Errorf("register existing v0.6 schema: %w", err)
				}
				continue
			}
		}
		script, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func databaseIsEmpty(ctx context.Context, database *sql.DB) (bool, error) {
	var count int
	err := database.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND table_name<>'schema_migrations'`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect database: %w", err)
	}
	return count == 0, nil
}

func validateV06Fingerprint(ctx context.Context, database *sql.DB) error {
	required := []string{"api_key_audits", "api_key_models", "api_keys", "mentor_project_applications", "model_bindings", "models", "organization_members", "organizations", "project_members", "projects", "providers", "users"}
	rows, err := database.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' AND table_name<>'schema_migrations' ORDER BY table_name`)
	if err != nil {
		return fmt.Errorf("inspect existing schema: %w", err)
	}
	defer rows.Close()
	actual := make([]string, 0, len(required))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("scan existing schema: %w", err)
		}
		actual = append(actual, table)
	}
	if strings.Join(actual, ",") != strings.Join(required, ",") {
		return fmt.Errorf("database schema is not the exact v0.6 baseline: found [%s]", strings.Join(actual, ", "))
	}
	var newColumns int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND ((table_name='projects' AND column_name='monthly_credit_quota_milli') OR (table_name='models' AND column_name='credit_multiplier_milli') OR (table_name='api_keys' AND column_name='requested_monthly_credit_quota_milli'))`).Scan(&newColumns); err != nil {
		return fmt.Errorf("inspect v0.7 columns: %w", err)
	}
	if newColumns != 0 {
		return fmt.Errorf("database contains a partial v0.7 schema")
	}
	return nil
}
