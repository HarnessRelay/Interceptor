package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/session"
)

// DB provides SQLite persistence for sessions and lightweight events.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite wal: %w", err)
	}
	s := &DB{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}
	return s, nil
}

func (s *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    name          TEXT,
    harness_type  TEXT,
    adapter_id    TEXT,
    adapter_name  TEXT,
    command       TEXT,
    args          TEXT,
    work_dir      TEXT,
    status        TEXT,
    exit_code     INTEGER,
    started_at    INTEGER,
    exited_at     INTEGER
);

CREATE TABLE IF NOT EXISTS events (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    type        TEXT NOT NULL,
    sequence    INTEGER NOT NULL,
    timestamp   INTEGER,
    data        TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_events_session_seq ON events(session_id, sequence);
CREATE INDEX IF NOT EXISTS idx_sessions_exited    ON sessions(exited_at DESC);
`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *DB) Close() error {
	return s.db.Close()
}

// ArchiveSession persists a session and its events to the archive.
func (s *DB) ArchiveSession(info session.Info, evts []events.Event) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	argsJSON, err := json.Marshal(info.Args)
	if err != nil {
		return err
	}

	var exitedAt any
	if info.ExitedAt != nil {
		exitedAt = info.ExitedAt.Unix()
	}

	var exitCode any
	if info.ExitCode != nil {
		exitCode = *info.ExitCode
	}

	_, err = tx.Exec(
		`INSERT OR REPLACE INTO sessions
		(id, name, harness_type, adapter_id, adapter_name, command, args, work_dir, status, exit_code, started_at, exited_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		info.ID,
		info.Name,
		info.HarnessType,
		info.AdapterID,
		info.AdapterName,
		info.Command,
		string(argsJSON),
		info.WorkDir,
		string(info.Status),
		exitCode,
		info.StartedAt.Unix(),
		exitedAt,
	)
	if err != nil {
		return err
	}

	for _, evt := range evts {
		if !isPersistedType(evt.Type) {
			continue
		}
		dataJSON, err := json.Marshal(evt.Data)
		if err != nil {
			continue
		}
		_, err = tx.Exec(
			`INSERT OR REPLACE INTO events
			(id, session_id, type, sequence, timestamp, data)
			VALUES (?, ?, ?, ?, ?, ?)`,
			evt.ID,
			evt.SessionID,
			string(evt.Type),
			evt.Sequence,
			evt.Timestamp.Unix(),
			string(dataJSON),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListArchivedSessions returns archived sessions sorted by exited_at DESC.
func (s *DB) ListArchivedSessions(limit int) ([]session.Info, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(
		`SELECT id, name, harness_type, adapter_id, adapter_name, command, args, work_dir, status, exit_code, started_at, exited_at
		FROM sessions
		ORDER BY exited_at DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []session.Info
	for rows.Next() {
		info, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, rows.Err()
}

// GetArchivedSession returns a single archived session by ID.
func (s *DB) GetArchivedSession(id string) (*session.Info, error) {
	row := s.db.QueryRow(
		`SELECT id, name, harness_type, adapter_id, adapter_name, command, args, work_dir, status, exit_code, started_at, exited_at
		FROM sessions WHERE id = ?`,
		id,
	)
	info, err := scanSession(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

// GetArchivedEvents returns stored events for a session.
func (s *DB) GetArchivedEvents(sessionID string, afterSeq uint64, limit int) ([]events.Event, error) {
	if limit <= 0 {
		limit = 1024
	}
	rows, err := s.db.Query(
		`SELECT id, type, sequence, timestamp, data FROM events
		WHERE session_id = ? AND sequence > ?
		ORDER BY sequence
		LIMIT ?`,
		sessionID, afterSeq, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []events.Event
	for rows.Next() {
		var evt events.Event
		var ts int64
		var dataJSON string
		var typ string
		err := rows.Scan(&evt.ID, &typ, &evt.Sequence, &ts, &dataJSON)
		if err != nil {
			return nil, err
		}
		evt.Type = events.Type(typ)
		evt.SessionID = sessionID
		evt.Timestamp = time.Unix(ts, 0)
		if dataJSON != "" {
			var raw any
			_ = json.Unmarshal([]byte(dataJSON), &raw)
			evt.Data = raw
		}
		out = append(out, evt)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(s scanner) (session.Info, error) {
	var info session.Info
	var argsJSON string
	var status string
	var exitCode sql.NullInt32
	var startedAt, exitedAt sql.NullInt64

	err := s.Scan(
		&info.ID,
		&info.Name,
		&info.HarnessType,
		&info.AdapterID,
		&info.AdapterName,
		&info.Command,
		&argsJSON,
		&info.WorkDir,
		&status,
		&exitCode,
		&startedAt,
		&exitedAt,
	)
	if err != nil {
		return session.Info{}, err
	}

	info.Status = session.Status(status)
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &info.Args)
	}
	if startedAt.Valid {
		info.StartedAt = time.Unix(startedAt.Int64, 0)
	}
	if exitedAt.Valid {
		t := time.Unix(exitedAt.Int64, 0)
		info.ExitedAt = &t
	}
	if exitCode.Valid {
		code := int(exitCode.Int32)
		info.ExitCode = &code
	}
	return info, nil
}

var persistedTypes = map[events.Type]bool{
	events.TypeChatUserMessage:     true,
	events.TypeChatAssistantMessage: true,
	events.TypeChatSystemMessage:   true,
	events.TypeApprovalRequired:    true,
	events.TypeApprovalResolved:    true,
	events.TypeHarnessStatus:       true,
	events.TypeHarnessMetadata:     true,
	events.TypeSessionStatusChanged: true,
	events.TypeSessionExited:       true,
}

func isPersistedType(typ events.Type) bool {
	return persistedTypes[typ]
}

// PersistEvent writes a single lightweight event to the archive DB.
// Terminal output and adapter warnings/errors are skipped.
func (s *DB) PersistEvent(evt events.Event) error {
	if !isPersistedType(evt.Type) {
		return nil
	}
	dataJSON, err := json.Marshal(evt.Data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO events (id, session_id, type, sequence, timestamp, data)
		VALUES (?, ?, ?, ?, ?, ?)`,
		evt.ID,
		evt.SessionID,
		string(evt.Type),
		evt.Sequence,
		evt.Timestamp.Unix(),
		string(dataJSON),
	)
	return err
}

// Info helpers for capabilities (stored empty in DB; restored as nil).
func capabilitiesFromStrings(capabilities []string) []harness.Capability {
	out := make([]harness.Capability, len(capabilities))
	for i, c := range capabilities {
		out[i] = harness.Capability(c)
	}
	return out
}
