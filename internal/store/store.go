// Package store persists the game journal and reads it back for replay and history.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Arseniy22877/game/migrations"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// Open opens a database file and applies any pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	query := url.Values{"_pragma": {
		"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)", "foreign_keys(1)",
	}}
	dsn.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if err := migrate(ctx, db, migrations.Files); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(ctx context.Context, db *sql.DB, files fs.FS) error {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return err
	}
	for i, name := range names {
		prefix, _, _ := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if err != nil || len(prefix) != 4 || version != i+1 {
			return fmt.Errorf("invalid migration name or sequence: %s", name)
		}
		err = transaction(ctx, db, func(tx *sql.Tx) error {
			var current int
			if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
				return err
			}
			if current >= version {
				return nil
			}
			query, err := fs.ReadFile(files, name)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, string(query)); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version))
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	return nil
}

func transaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// execOne treats a stale transition or missing row as an error, including in a batch.
func execOne(ctx context.Context, db executor, query string, args ...any) error {
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func encode(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func array[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
