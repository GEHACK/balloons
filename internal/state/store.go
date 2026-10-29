// Package state persists per-balloon ticket lifecycle in a local SQLite file:
// when the printer accepted the ticket (printed_at) and when the balloon was
// marked done in DOMjudge through our MarkDone RPC (delivered_at). It is the
// source of truth for "have we already printed this balloon?", which lets the
// hub safely reprint nothing on restart and still catch balloons inserted
// while the server was down.
package state

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Record struct {
	BalloonID   int64
	PrintedAt   *time.Time
	DeliveredAt *time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS ticket_state (
    balloon_id   INTEGER PRIMARY KEY,
    printed_at   TEXT,
    delivered_at TEXT
);
CREATE TABLE IF NOT EXISTS print_jobs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    team_name     TEXT NOT NULL,
    requesting_ip TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    delivered_at  TEXT
);
`

// Open opens (or creates) the SQLite database at path and ensures the schema
// exists. The returned Store is safe for concurrent use.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("state: open %q: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state: schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// IsPrinted reports whether ticket_state has a non-null printed_at for this id.
func (s *Store) IsPrinted(balloonID int64) (bool, error) {
	var has int
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM ticket_state WHERE balloon_id=? AND printed_at IS NOT NULL)`,
		balloonID,
	).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("state: IsPrinted(%d): %w", balloonID, err)
	}
	return has == 1, nil
}

// RecordPrinted sets printed_at = now() for this balloon, creating the row if
// it doesn't exist. Idempotent re-calls overwrite the timestamp; that's fine
// because IsPrinted gates upstream so we only get here for a fresh print.
func (s *Store) RecordPrinted(balloonID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO ticket_state (balloon_id, printed_at) VALUES (?, ?)
		 ON CONFLICT(balloon_id) DO UPDATE SET printed_at=excluded.printed_at`,
		balloonID, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("state: RecordPrinted(%d): %w", balloonID, err)
	}
	return nil
}

// ClearPrinted nulls printed_at for this balloon so the next print attempt is
// allowed through. Used to support manual "reprint" requests for tickets that
// were lost in transit. No-op if the row doesn't exist.
func (s *Store) ClearPrinted(balloonID int64) error {
	_, err := s.db.Exec(`UPDATE ticket_state SET printed_at=NULL WHERE balloon_id=?`, balloonID)
	if err != nil {
		return fmt.Errorf("state: ClearPrinted(%d): %w", balloonID, err)
	}
	return nil
}

// RecordDelivered sets delivered_at = now(), creating the row if needed. Called
// after a successful MarkDone against DOMjudge.
func (s *Store) RecordDelivered(balloonID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO ticket_state (balloon_id, delivered_at) VALUES (?, ?)
		 ON CONFLICT(balloon_id) DO UPDATE SET delivered_at=excluded.delivered_at`,
		balloonID, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("state: RecordDelivered(%d): %w", balloonID, err)
	}
	return nil
}

// Get returns the ticket_state row for balloonID. The second return value is
// false if no row exists. Useful for diagnostics; not on the hot path.
func (s *Store) Get(balloonID int64) (Record, bool, error) {
	var r Record
	r.BalloonID = balloonID
	var printed, delivered sql.NullString
	err := s.db.QueryRow(
		`SELECT printed_at, delivered_at FROM ticket_state WHERE balloon_id=?`,
		balloonID,
	).Scan(&printed, &delivered)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("state: Get(%d): %w", balloonID, err)
	}
	if printed.Valid {
		t, _ := time.Parse(time.RFC3339Nano, printed.String)
		r.PrintedAt = &t
	}
	if delivered.Valid {
		t, _ := time.Parse(time.RFC3339Nano, delivered.String)
		r.DeliveredAt = &t
	}
	return r, true, nil
}

// PrintJob is one row of print_jobs: a team print that went through cuproxy.
// Unlike balloons these exist nowhere but here, so this table is their source
// of truth, not a cache.
type PrintJob struct {
	ID           int64
	TeamName     string
	RequestingIP string
	Delivered    bool
}

// CreatePrintJob inserts a new, undelivered print job and returns it with its
// freshly assigned id.
func (s *Store) CreatePrintJob(teamName, requestingIP string) (PrintJob, error) {
	res, err := s.db.Exec(
		`INSERT INTO print_jobs (team_name, requesting_ip, created_at) VALUES (?, ?, ?)`,
		teamName, requestingIP, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return PrintJob{}, fmt.Errorf("state: CreatePrintJob: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return PrintJob{}, fmt.Errorf("state: CreatePrintJob: last id: %w", err)
	}
	return PrintJob{ID: id, TeamName: teamName, RequestingIP: requestingIP}, nil
}

// RecordPrintJobDelivered sets delivered_at = now() for the print job. No-op
// if the id doesn't exist.
func (s *Store) RecordPrintJobDelivered(id int64) error {
	_, err := s.db.Exec(
		`UPDATE print_jobs SET delivered_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("state: RecordPrintJobDelivered(%d): %w", id, err)
	}
	return nil
}

// ListPrintJobs returns every print job, oldest first. Called once at startup
// to seed the hub.
func (s *Store) ListPrintJobs() ([]PrintJob, error) {
	rows, err := s.db.Query(
		`SELECT id, team_name, requesting_ip, delivered_at IS NOT NULL FROM print_jobs ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("state: ListPrintJobs: %w", err)
	}
	defer rows.Close()
	var out []PrintJob
	for rows.Next() {
		var j PrintJob
		if err := rows.Scan(&j.ID, &j.TeamName, &j.RequestingIP, &j.Delivered); err != nil {
			return nil, fmt.Errorf("state: ListPrintJobs: scan: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListPrintJobs: %w", err)
	}
	return out, nil
}
