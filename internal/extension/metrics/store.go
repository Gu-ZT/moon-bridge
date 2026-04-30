// Package metrics implements metrics persistence using the foundation/db Store.
package metrics

import (
	"context"
	"fmt"
	"time"

	"moonbridge/internal/foundation/db"
)

// Record represents a single request metric row.
type Record struct {
	ID            int64         `json:"id"`
	Timestamp     time.Time     `json:"timestamp"`
	Model         string        `json:"model"`
	ActualModel   string        `json:"actual_model"`
	InputTokens   int64         `json:"input_tokens"`
	OutputTokens  int64         `json:"output_tokens"`
	CacheCreation int64         `json:"cache_creation"`
	CacheRead     int64         `json:"cache_read"`
	Cost          float64       `json:"cost"`
	ResponseTime  time.Duration `json:"response_time"`
	Status        string        `json:"status"`
	ErrorMessage  string        `json:"error_message,omitempty"`
}

// QueryOptions controls filtering and ordering for Record queries.
type QueryOptions struct {
	Limit    int
	Offset   int
	Since    time.Time
	Until    time.Time
	Model    string
	Status   string
	OrderAsc bool
}

// Store provides metrics CRUD operations backed by a foundation/db.Store.
type Store struct {
	s db.Store
}

// NewStore creates a metrics Store backed by the given database Store.
func NewStore(s db.Store) *Store {
	return &Store{s: s}
}

// MetricsTable returns the TableSpec for the metrics extension.
func MetricsTable() db.TableSpec {
	return db.TableSpec{
		Name: "request_metrics",
		Schema: `CREATE TABLE IF NOT EXISTS {{table}} (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp       TEXT    NOT NULL,
			model           TEXT    NOT NULL DEFAULT '',
			actual_model    TEXT    NOT NULL DEFAULT '',
			input_tokens    INTEGER NOT NULL DEFAULT 0,
			output_tokens   INTEGER NOT NULL DEFAULT 0,
			cache_creation  INTEGER NOT NULL DEFAULT 0,
			cache_read      INTEGER NOT NULL DEFAULT 0,
			cost            REAL    NOT NULL DEFAULT 0.0,
			response_time_ms INTEGER NOT NULL DEFAULT 0,
			status          TEXT    NOT NULL DEFAULT 'success',
			error_message   TEXT    NOT NULL DEFAULT ''
		)`,
		Indexes: []db.IndexSpec{
			{Name: "timestamp", SQL: `CREATE INDEX IF NOT EXISTS {{index}} ON {{table}}(timestamp)`},
			{Name: "model", SQL: `CREATE INDEX IF NOT EXISTS {{index}} ON {{table}}(model)`},
			{Name: "status", SQL: `CREATE INDEX IF NOT EXISTS {{index}} ON {{table}}(status)`},
		},
	}
}

// Record inserts a request metric row.
func (s *Store) Record(r Record) error {
	if s == nil || s.s == nil {
		return nil
	}
	table, err := s.s.Table("request_metrics")
	if err != nil {
		return err
	}
	ms := r.ResponseTime.Milliseconds()
	ts := r.Timestamp.UTC().Format(time.RFC3339Nano)
	query := fmt.Sprintf(`INSERT INTO %s
		(timestamp, model, actual_model, input_tokens, output_tokens,
		 cache_creation, cache_read, cost, response_time_ms, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, table)
	_, err = s.s.ExecContext(context.Background(), query,
		ts, r.Model, r.ActualModel,
		r.InputTokens, r.OutputTokens, r.CacheCreation, r.CacheRead,
		r.Cost, ms, r.Status, r.ErrorMessage,
	)
	return err
}

// Query retrieves request metrics matching the given options.
func (s *Store) Query(opts QueryOptions) ([]Record, error) {
	if s == nil || s.s == nil {
		return nil, nil
	}
	table, err := s.s.Table("request_metrics")
	if err != nil {
		return nil, err
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	var whereClauses []string
	var args []any

	if !opts.Since.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	if !opts.Until.IsZero() {
		whereClauses = append(whereClauses, "timestamp < ?")
		args = append(args, opts.Until.UTC().Format(time.RFC3339Nano))
	}
	if opts.Model != "" {
		whereClauses = append(whereClauses, "model = ?")
		args = append(args, opts.Model)
	}
	if opts.Status != "" {
		whereClauses = append(whereClauses, "status = ?")
		args = append(args, opts.Status)
	}

	order := "DESC"
	if opts.OrderAsc {
		order = "ASC"
	}

	query := fmt.Sprintf("SELECT id, timestamp, model, actual_model, input_tokens, output_tokens, "+
		"cache_creation, cache_read, cost, response_time_ms, status, error_message "+
		"FROM %s", table)
	if len(whereClauses) > 0 {
		query += " WHERE " + joinClauses(whereClauses)
	}
	query += " ORDER BY id " + order + " LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.s.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		var ts string
		var ms int64
		err := rows.Scan(
			&r.ID, &ts, &r.Model, &r.ActualModel,
			&r.InputTokens, &r.OutputTokens, &r.CacheCreation, &r.CacheRead,
			&r.Cost, &ms, &r.Status, &r.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("scan metrics row: %w", err)
		}
		r.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		r.ResponseTime = time.Duration(ms) * time.Millisecond
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metrics rows: %w", err)
	}
	return records, nil
}

func joinClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += " AND "
		}
		result += c
	}
	return result
}
