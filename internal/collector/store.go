// Package collector stores results in SQLite and serves the API and web UI.
package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"smtbench/internal/result"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

const schema = `CREATE TABLE IF NOT EXISTS runs (
	id           TEXT PRIMARY KEY,
	received_at  TEXT NOT NULL,
	label        TEXT NOT NULL,
	hostname     TEXT NOT NULL,
	cpu_model    TEXT NOT NULL,
	os           TEXT NOT NULL,
	profile      TEXT NOT NULL,
	tool_version TEXT NOT NULL,
	started_at   TEXT NOT NULL,
	partial      INTEGER NOT NULL,
	raw          BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS runs_started ON runs(started_at);`

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("создание схемы БД: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Put inserts or replaces a run; created reports whether it was new.
func (s *Store) Put(ctx context.Context, r *result.Run, raw []byte) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE id = ?`, r.RunID).Scan(&n); err != nil {
		return false, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO runs (id, received_at, label, hostname, cpu_model, os, profile, tool_version, started_at, partial, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET label=excluded.label, hostname=excluded.hostname, cpu_model=excluded.cpu_model,
			os=excluded.os, profile=excluded.profile, tool_version=excluded.tool_version, started_at=excluded.started_at,
			partial=excluded.partial, raw=excluded.raw`,
		r.RunID, time.Now().UTC().Format(time.RFC3339Nano), r.Label, r.Hostname, r.CPU.Model, r.OS, r.Params.Profile,
		r.ToolVersion, r.StartedAt.UTC().Format(time.RFC3339Nano), r.Partial, raw)
	return n == 0, err
}

type row struct {
	Meta
	raw []byte
}

func (s *Store) list(ctx context.Context) ([]row, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, hostname, cpu_model, os, profile, tool_version, started_at, received_at, partial, raw
		FROM runs ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []row
	for rows.Next() {
		var x row
		var started, received string
		if err := rows.Scan(&x.ID, &x.Label, &x.Hostname, &x.CPUModel, &x.OS, &x.Profile, &x.ToolVersion, &started, &received, &x.Partial, &x.raw); err != nil {
			return nil, err
		}
		x.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		x.ReceivedAt, _ = time.Parse(time.RFC3339Nano, received)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) ([]byte, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT raw FROM runs WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}
