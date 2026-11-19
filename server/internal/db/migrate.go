package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migration describes one versioned SQL migration embedded in the binary.
// Versions are four-digit, zero-padded prefixes so lexical sorting is the same
// as numeric ordering.
type Migration struct {
	Version  string
	Name     string
	SQL      string
	Checksum string
}

const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

var migrationFilePattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migrate applies embedded migrations in version order inside one transaction.
//
// Installations created before versioned migrations (the historical idempotent
// schema.sql) are detected by the presence of the tenants table and are marked
// as already having the baseline migration applied before newer migrations run.
// This keeps upgrades and fresh installs on the same code path.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, schemaMigrationsDDL); err != nil {
		return 0, fmt.Errorf("create schema_migrations table: %w", err)
	}

	var recorded int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&recorded); err != nil {
		return 0, fmt.Errorf("count applied migrations: %w", err)
	}
	if recorded == 0 {
		var legacySchema bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass('public.tenants') IS NOT NULL`).Scan(&legacySchema); err != nil {
			return 0, fmt.Errorf("detect legacy schema: %w", err)
		}
		if legacySchema {
			baseline := migrations[0]
			if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`, baseline.Version, baseline.Name, baseline.Checksum); err != nil {
				return 0, fmt.Errorf("record legacy baseline migration %s: %w", baseline.Version, err)
			}
		}
	}

	applied := make(map[string]string, len(migrations))
	rows, err := tx.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return 0, fmt.Errorf("load applied migrations: %w", err)
	}
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate applied migrations: %w", err)
	}
	rows.Close()

	count := 0
	for _, migration := range migrations {
		if checksum, ok := applied[migration.Version]; ok {
			if checksum != migration.Checksum {
				return 0, fmt.Errorf("migration %s (%s) changed after being applied; add a new migration instead of editing it", migration.Version, migration.Name)
			}
			continue
		}
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return 0, fmt.Errorf("apply migration %s (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			return 0, fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
		applied[migration.Version] = migration.Checksum
		count++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit migrations: %w", err)
	}
	return count, nil
}

func loadMigrations() ([]Migration, error) {
	return parseMigrations(migrationFiles, "migrations")
}

// parseMigrations reads and validates a migration directory. It is separate
// from the embed so tests can exercise naming, ordering, and duplicate rules.
func parseMigrations(fsys fs.FS, root string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	seen := make(map[string]string, len(entries))
	items := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		match := migrationFilePattern.FindStringSubmatch(fileName)
		if match == nil {
			return nil, fmt.Errorf("invalid migration file %q: expected 0001_snake_case.sql", fileName)
		}
		version := match[1]
		if existing, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %s: %s and %s", version, existing, fileName)
		}
		seen[version] = fileName

		content, err := fs.ReadFile(fsys, path.Join(root, fileName))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", fileName, err)
		}
		sum := sha256.Sum256(content)
		items = append(items, Migration{
			Version:  version,
			Name:     strings.TrimSuffix(fileName, ".sql"),
			SQL:      string(content),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	if len(items) == 0 {
		return nil, errors.New("no migrations found")
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Version < items[j].Version })
	return items, nil
}
