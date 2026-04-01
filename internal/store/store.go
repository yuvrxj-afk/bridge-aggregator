package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"bridge-aggregator/internal/models"

	_ "github.com/lib/pq"
)

// Store wraps a PostgreSQL-backed store for operations.
type Store struct {
	DB *sql.DB
}

var ErrNotFound = errors.New("not found")

// Operation represents a bridge operation we track.
type Operation struct {
	ID                string
	Route             models.Route
	Status            string
	ClientReferenceID string
	IdempotencyKey    string
	TxHash            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OperationEvent is an immutable event entry for operation lifecycle changes.
type OperationEvent struct {
	ID          int64
	OperationID string
	EventType   string
	FromStatus  string
	ToStatus    string
	TxHash      string
	Metadata    string
	CreatedAt   time.Time
}

// NewStore connects to Postgres and ensures the schema exists.
func NewStore(databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Store{DB: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) initSchema() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS operations (
  id TEXT PRIMARY KEY,
  route JSONB NOT NULL,
  status TEXT NOT NULL,
  client_reference_id TEXT,
  idempotency_key TEXT,
  tx_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS operations_idempotency_key_idx
  ON operations(idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS operation_events (
  id BIGSERIAL PRIMARY KEY,
  operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT,
  tx_hash TEXT,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS operation_events_operation_id_idx
  ON operation_events(operation_id, created_at DESC);

-- Migration: add tx_hash column to existing deployments (safe no-op if already present).
ALTER TABLE operations ADD COLUMN IF NOT EXISTS tx_hash TEXT;
`
	_, err := s.DB.Exec(ddl)
	return err
}

// CreateOperation inserts a new operation, enforcing idempotency if key is non-empty.
func (s *Store) CreateOperation(op Operation) (*Operation, error) {
	if op.IdempotencyKey != "" {
		if existing, err := s.GetOperationByIdempotencyKey(op.IdempotencyKey); err == nil && existing != nil {
			return existing, nil
		}
	}

	routeBytes, err := json.Marshal(op.Route)
	if err != nil {
		return nil, err
	}

	const q = `
INSERT INTO operations (id, route, status, client_reference_id, idempotency_key)
VALUES ($1, $2, $3, $4, $5)
RETURNING created_at, updated_at;
`
	row := s.DB.QueryRow(q, op.ID, routeBytes, op.Status, nullIfEmpty(op.ClientReferenceID), nullIfEmpty(op.IdempotencyKey))
	if err := row.Scan(&op.CreatedAt, &op.UpdatedAt); err != nil {
		return nil, err
	}
	if err := s.AppendOperationEvent(op.ID, "created", "", op.Status, "", `{"source":"execute"}`); err != nil {
		return nil, err
	}
	return &op, nil
}

// UpdateOperationStatus sets the status (and optionally tx_hash) for an operation.
// Valid statuses: pending, submitted, completed, failed.
func (s *Store) UpdateOperationStatus(id, status, txHash string) error {
	cur, err := s.GetOperation(id)
	if err != nil {
		return err
	}
	if cur == nil {
		return sql.ErrNoRows
	}

	const q = `
UPDATE operations
SET status = $2, tx_hash = COALESCE(NULLIF($3, ''), tx_hash), updated_at = now()
WHERE id = $1;
`
	res, err := s.DB.Exec(q, id, status, txHash)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if err := s.AppendOperationEvent(id, "status_transition", cur.Status, status, txHash, `{"source":"patch_status"}`); err != nil {
		return err
	}
	return nil
}

// GetOperation fetches an operation by ID.
func (s *Store) GetOperation(id string) (*Operation, error) {
	const q = `
SELECT id, route, status, client_reference_id, idempotency_key, tx_hash, created_at, updated_at
FROM operations
WHERE id = $1;
`
	var (
		row   Operation
		rjson []byte
	)
	if err := s.DB.QueryRow(q, id).Scan(
		&row.ID,
		&rjson,
		&row.Status,
		&row.ClientReferenceID,
		&row.IdempotencyKey,
		&row.TxHash,
		&row.CreatedAt,
		&row.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(rjson, &row.Route); err != nil {
		return nil, err
	}
	return &row, nil
}

// GetOperationByIdempotencyKey fetches an operation by idempotency key.
func (s *Store) GetOperationByIdempotencyKey(key string) (*Operation, error) {
	if key == "" {
		return nil, nil
	}
	const q = `
SELECT id, route, status, client_reference_id, idempotency_key, tx_hash, created_at, updated_at
FROM operations
WHERE idempotency_key = $1
LIMIT 1;
`
	var (
		row   Operation
		rjson []byte
	)
	if err := s.DB.QueryRow(q, key).Scan(
		&row.ID,
		&rjson,
		&row.Status,
		&row.ClientReferenceID,
		&row.IdempotencyKey,
		&row.TxHash,
		&row.CreatedAt,
		&row.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(rjson, &row.Route); err != nil {
		return nil, err
	}
	return &row, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AppendOperationEvent appends an immutable lifecycle event.
func (s *Store) AppendOperationEvent(operationID, eventType, fromStatus, toStatus, txHash, metadata string) error {
	const q = `
INSERT INTO operation_events (operation_id, event_type, from_status, to_status, tx_hash, metadata)
VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,'')::jsonb);
`
	_, err := s.DB.Exec(q, operationID, eventType, fromStatus, toStatus, txHash, metadata)
	return err
}

// ListOperationEvents returns operation lifecycle events newest-first.
func (s *Store) ListOperationEvents(operationID string, limit int) ([]OperationEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
SELECT id, operation_id, event_type, COALESCE(from_status,''), COALESCE(to_status,''), COALESCE(tx_hash,''), COALESCE(metadata::text,'{}'), created_at
FROM operation_events
WHERE operation_id = $1
ORDER BY created_at DESC
LIMIT $2;
`
	rows, err := s.DB.Query(q, operationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]OperationEvent, 0, limit)
	for rows.Next() {
		var e OperationEvent
		if err := rows.Scan(&e.ID, &e.OperationID, &e.EventType, &e.FromStatus, &e.ToStatus, &e.TxHash, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
