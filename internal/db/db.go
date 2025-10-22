package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrationsSQL string

type DB struct {
	SQL *sql.DB
}

func Open(dataDir string) (*DB, error) {
	path := filepath.Join(dataDir, "kill-the-newsletter.db")
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Pragmas for reliability.
	if _, err := d.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = d.Close()
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := migrate(d); err != nil {
		_ = d.Close()
		return nil, err
	}
	return &DB{SQL: d}, nil
}

func (d *DB) Close() error { return d.SQL.Close() }

func migrate(d *sql.DB) error {
	// Single-shot migration using embedded SQL; idempotent via IF NOT EXISTS and CREATE UNIQUE indices.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := d.ExecContext(ctx, migrationsSQL)
	return err
}

// Tx wraps a function in a transaction.
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func ErrNotFound(entity string) error { return fmt.Errorf("%s not found", entity) }
