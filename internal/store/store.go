package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/loreteam/lore/internal/ticket"
)

// Store is a SQLite-backed index of ticket data.
// It is entirely disposable — lore doctor rebuilds it from git refs.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

const schema = `
CREATE TABLE IF NOT EXISTS tickets (
	id          TEXT PRIMARY KEY,
	sha         TEXT NOT NULL,
	title       TEXT NOT NULL,
	status      TEXT NOT NULL,
	priority    INTEGER NOT NULL DEFAULT 3,
	agent       TEXT NOT NULL DEFAULT '',
	parent      TEXT NOT NULL DEFAULT '',
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ticket_files (
	ticket_id   TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
	file_path   TEXT NOT NULL,
	PRIMARY KEY (ticket_id, file_path)
);
CREATE TABLE IF NOT EXISTS questions (
	id           TEXT PRIMARY KEY,
	ticket_id    TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
	type         TEXT NOT NULL,
	text         TEXT NOT NULL,
	blocking     INTEGER NOT NULL DEFAULT 0,
	assumption   TEXT NOT NULL DEFAULT '',
	directed_to  TEXT NOT NULL DEFAULT '',
	answer       TEXT NOT NULL DEFAULT '',
	answered_by  TEXT NOT NULL DEFAULT '',
	asked_at     TEXT NOT NULL,
	answered_at  TEXT
);
CREATE TABLE IF NOT EXISTS embeddings (
	ticket_id   TEXT PRIMARY KEY REFERENCES tickets(id) ON DELETE CASCADE,
	sha         TEXT NOT NULL,
	vector      BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tickets_status      ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_tickets_priority    ON tickets(priority DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_created_at  ON tickets(created_at);
CREATE INDEX IF NOT EXISTS idx_questions_ticket_id ON questions(ticket_id);
CREATE INDEX IF NOT EXISTS idx_questions_directed  ON questions(directed_to);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// UpsertTicket inserts or updates a ticket row and its associated file rows.
func (s *Store) UpsertTicket(t *ticket.Ticket, sha string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: upsert begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO tickets (id, sha, title, status, priority, agent, parent, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			sha=excluded.sha, title=excluded.title, status=excluded.status,
			priority=excluded.priority, agent=excluded.agent, parent=excluded.parent,
			updated_at=excluded.updated_at
	`, t.ID, sha, t.Title, string(t.Status), t.Priority,
		t.Agent, t.Parent,
		t.CreatedAt.UTC().Format(time.RFC3339),
		t.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: upsert ticket: %w", err)
	}

	if _, err = tx.Exec(`DELETE FROM ticket_files WHERE ticket_id = ?`, t.ID); err != nil {
		return fmt.Errorf("store: clear files: %w", err)
	}
	for _, f := range t.Files {
		if _, err = tx.Exec(`INSERT INTO ticket_files (ticket_id, file_path) VALUES (?, ?)`, t.ID, f); err != nil {
			return fmt.Errorf("store: insert file: %w", err)
		}
	}

	if _, err = tx.Exec(`DELETE FROM questions WHERE ticket_id = ?`, t.ID); err != nil {
		return fmt.Errorf("store: clear questions: %w", err)
	}
	for _, q := range t.Questions {
		var answeredAt *string
		if q.AnsweredAt != nil {
			s := q.AnsweredAt.UTC().Format(time.RFC3339)
			answeredAt = &s
		}
		blocking := 0
		if q.Blocking {
			blocking = 1
		}
		_, err = tx.Exec(`
			INSERT INTO questions
			  (id, ticket_id, type, text, blocking, assumption, directed_to,
			   answer, answered_by, asked_at, answered_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.ID, t.ID, string(q.Type), q.Text, blocking,
			q.Assumption, q.DirectedTo, q.Answer, q.AnsweredBy,
			q.AskedAt.UTC().Format(time.RFC3339), answeredAt,
		)
		if err != nil {
			return fmt.Errorf("store: insert question: %w", err)
		}
	}

	return tx.Commit()
}

// Row is a lightweight ticket record returned by list queries.
type Row struct {
	ID        string
	SHA       string
	Title     string
	Status    string
	Priority  int
	Agent     string
	Parent    string
	CreatedAt string
	UpdatedAt string
}

// ListFilter controls which tickets ListTickets returns.
type ListFilter struct {
	Status string // empty = all statuses
}

// ListTickets returns tickets matching the filter, ordered by priority desc then updated_at desc.
func (s *Store) ListTickets(f ListFilter) ([]*Row, error) {
	q := `SELECT id, sha, title, status, priority, agent, parent, created_at, updated_at FROM tickets`
	var args []any
	if f.Status != "" {
		q += ` WHERE status = ?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY priority DESC, updated_at DESC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list tickets: %w", err)
	}
	defer rows.Close()

	var result []*Row
	for rows.Next() {
		r := &Row{}
		if err := rows.Scan(&r.ID, &r.SHA, &r.Title, &r.Status, &r.Priority, &r.Agent, &r.Parent, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan ticket: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// QuestionRow is a lightweight question record returned by list queries.
type QuestionRow struct {
	ID         string
	TicketID   string
	Type       string
	Text       string
	Blocking   bool
	Assumption string
	DirectedTo string
	Answer     string
	AnsweredBy string
	AskedAt    string
	AnsweredAt *string // nil means unanswered
}

// QuestionFilter controls which questions ListQuestions returns.
type QuestionFilter struct {
	Unanswered   bool   // only questions with no answer yet
	Answered     bool   // only questions that have been answered
	BlockingOnly bool   // only blocking questions
	DirectedTo   string // only questions directed at this agent/role
	TicketID     string // only questions for this ticket
}

// ListQuestions returns questions matching the filter, ordered by asked_at ASC.
func (s *Store) ListQuestions(f QuestionFilter) ([]*QuestionRow, error) {
	q := `SELECT id, ticket_id, type, text, blocking, assumption, directed_to,
	             answer, answered_by, asked_at, answered_at
	      FROM questions`
	var args []any
	var where []string
	if f.Unanswered {
		where = append(where, `answered_at IS NULL`)
	}
	if f.Answered {
		where = append(where, `answered_at IS NOT NULL`)
	}
	if f.BlockingOnly {
		where = append(where, `blocking = 1`)
	}
	if f.DirectedTo != "" {
		where = append(where, `directed_to = ?`)
		args = append(args, f.DirectedTo)
	}
	if f.TicketID != "" {
		where = append(where, `ticket_id = ?`)
		args = append(args, f.TicketID)
	}
	if len(where) > 0 {
		q += ` WHERE ` + where[0]
		for _, w := range where[1:] {
			q += ` AND ` + w
		}
	}
	q += ` ORDER BY asked_at ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list questions: %w", err)
	}
	defer rows.Close()

	var result []*QuestionRow
	for rows.Next() {
		r := &QuestionRow{}
		var blocking int
		if err := rows.Scan(
			&r.ID, &r.TicketID, &r.Type, &r.Text, &blocking,
			&r.Assumption, &r.DirectedTo, &r.Answer, &r.AnsweredBy,
			&r.AskedAt, &r.AnsweredAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan question: %w", err)
		}
		r.Blocking = blocking == 1
		result = append(result, r)
	}
	return result, rows.Err()
}

// CountQuestions returns the count of questions matching the filter.
func (s *Store) CountQuestions(f QuestionFilter) (int, error) {
	q := `SELECT COUNT(*) FROM questions`
	var args []any
	var where []string
	if f.Unanswered {
		where = append(where, `answered_at IS NULL`)
	}
	if f.Answered {
		where = append(where, `answered_at IS NOT NULL`)
	}
	if f.BlockingOnly {
		where = append(where, `blocking = 1`)
	}
	if f.DirectedTo != "" {
		where = append(where, `directed_to = ?`)
		args = append(args, f.DirectedTo)
	}
	if f.TicketID != "" {
		where = append(where, `ticket_id = ?`)
		args = append(args, f.TicketID)
	}
	if len(where) > 0 {
		q += ` WHERE ` + where[0]
		for _, w := range where[1:] {
			q += ` AND ` + w
		}
	}
	var count int
	if err := s.db.QueryRow(q, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count questions: %w", err)
	}
	return count, nil
}

// DeleteTicket removes a ticket row (and its files via cascade).
func (s *Store) DeleteTicket(id string) error {
	_, err := s.db.Exec(`DELETE FROM tickets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete ticket %s: %w", id, err)
	}
	return nil
}

// scanRows drains a *sql.Rows cursor into a []*Row slice.
func scanRows(rows *sql.Rows) ([]*Row, error) {
	var result []*Row
	for rows.Next() {
		r := &Row{}
		if err := rows.Scan(&r.ID, &r.SHA, &r.Title, &r.Status, &r.Priority, &r.Agent, &r.Parent, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// FindRelatedByFiles returns tickets that share at least one file with ticketID,
// excluding ticketID itself. Results are ordered by priority desc, updated_at desc.
func (s *Store) FindRelatedByFiles(ticketID string) ([]*Row, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT t.id, t.sha, t.title, t.status, t.priority, t.agent, t.parent, t.created_at, t.updated_at
		FROM tickets t
		JOIN ticket_files tf ON tf.ticket_id = t.id
		WHERE t.id != ?
		  AND tf.file_path IN (SELECT file_path FROM ticket_files WHERE ticket_id = ?)
		ORDER BY t.priority DESC, t.updated_at DESC`,
		ticketID, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: find related %s: %w", ticketID, err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// FindTicketsByFile returns all tickets that reference filePath, ordered by priority desc, updated_at desc.
func (s *Store) FindTicketsByFile(filePath string) ([]*Row, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.sha, t.title, t.status, t.priority, t.agent, t.parent, t.created_at, t.updated_at
		FROM tickets t
		JOIN ticket_files tf ON tf.ticket_id = t.id
		WHERE tf.file_path = ?
		ORDER BY t.priority DESC, t.updated_at DESC`,
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("store: find by file %s: %w", filePath, err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// AgentRow summarises activity for a single agent.
type AgentRow struct {
	Agent      string
	Total      int
	Open       int
	InProgress int
	Blocked    int
	Ready      int
}

// ListAgents returns one AgentRow per distinct non-empty agent value, sorted by in_progress desc.
// If contextFile is non-empty, only tickets touching that file are included.
func (s *Store) ListAgents(contextFile string) ([]AgentRow, error) {
	const selectPart = `
		SELECT agent,
		       COUNT(*) AS total,
		       SUM(CASE WHEN status='open'        THEN 1 ELSE 0 END),
		       SUM(CASE WHEN status='in-progress' THEN 1 ELSE 0 END) AS wip,
		       SUM(CASE WHEN status='blocked'     THEN 1 ELSE 0 END),
		       SUM(CASE WHEN status='ready'       THEN 1 ELSE 0 END)
		FROM tickets
		WHERE agent != ''`
	const orderPart = `
		GROUP BY agent
		ORDER BY wip DESC`

	var (
		q    string
		args []any
	)
	if contextFile != "" {
		q = selectPart + `
		  AND id IN (SELECT ticket_id FROM ticket_files WHERE file_path = ?)` + orderPart
		args = append(args, contextFile)
	} else {
		q = selectPart + orderPart
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list agents: %w", err)
	}
	defer rows.Close()

	var result []AgentRow
	for rows.Next() {
		var r AgentRow
		if err := rows.Scan(&r.Agent, &r.Total, &r.Open, &r.InProgress, &r.Blocked, &r.Ready); err != nil {
			return nil, fmt.Errorf("store: scan agent: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// StatusCounts holds ticket counts per status.
type StatusCounts struct {
	Open       int
	InProgress int
	Blocked    int
	Ready      int
	Closed     int
}

// CountByStatus returns ticket counts grouped by status.
func (s *Store) CountByStatus() (StatusCounts, error) {
	rows, err := s.db.Query(`
		SELECT status, COUNT(*) FROM tickets GROUP BY status`)
	if err != nil {
		return StatusCounts{}, fmt.Errorf("store: count by status: %w", err)
	}
	defer rows.Close()

	var c StatusCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return StatusCounts{}, fmt.Errorf("store: scan count: %w", err)
		}
		switch ticket.Status(status) {
		case ticket.StatusOpen:
			c.Open = count
		case ticket.StatusInProgress:
			c.InProgress = count
		case ticket.StatusBlocked:
			c.Blocked = count
		case ticket.StatusReady:
			c.Ready = count
		case ticket.StatusClosed:
			c.Closed = count
		}
	}
	return c, rows.Err()
}

// OldestOpen returns the Row for the oldest non-closed ticket, or nil if none.
func (s *Store) OldestOpen() (*Row, error) {
	r := &Row{}
	err := s.db.QueryRow(`
		SELECT id, sha, title, status, priority, agent, parent, created_at, updated_at
		FROM tickets
		WHERE status != ?
		ORDER BY created_at ASC
		LIMIT 1`,
		string(ticket.StatusClosed),
	).Scan(&r.ID, &r.SHA, &r.Title, &r.Status, &r.Priority, &r.Agent, &r.Parent, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: oldest open: %w", err)
	}
	return r, nil
}

// RecentlyUpdated returns tickets updated within the last n days, ordered by updated_at desc.
func (s *Store) RecentlyUpdated(days int) ([]*Row, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT id, sha, title, status, priority, agent, parent, created_at, updated_at
		FROM tickets
		WHERE updated_at >= ?
		ORDER BY updated_at DESC`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("store: recently updated: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// GetEmbedding returns the raw encoded vector for ticketID if sha matches.
// Returns nil, nil when no fresh cache entry exists (missing or stale SHA).
func (s *Store) GetEmbedding(ticketID, sha string) ([]byte, error) {
	var storedSHA string
	var blob []byte
	err := s.db.QueryRow(
		`SELECT sha, vector FROM embeddings WHERE ticket_id = ?`, ticketID,
	).Scan(&storedSHA, &blob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get embedding %s: %w", ticketID, err)
	}
	if storedSHA != sha {
		return nil, nil
	}
	return blob, nil
}

// SetEmbedding upserts the raw encoded vector for ticketID with the given sha.
func (s *Store) SetEmbedding(ticketID, sha string, blob []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO embeddings (ticket_id, sha, vector) VALUES (?, ?, ?)
		ON CONFLICT(ticket_id) DO UPDATE SET sha=excluded.sha, vector=excluded.vector`,
		ticketID, sha, blob,
	)
	if err != nil {
		return fmt.Errorf("store: set embedding %s: %w", ticketID, err)
	}
	return nil
}
