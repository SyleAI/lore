package ticket

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Status represents the lifecycle state of a ticket.
type Status string

const (
	StatusOpen            Status = "open"
	StatusWorking         Status = "working"
	StatusBlocked         Status = "blocked"
	StatusReadyForReview  Status = "ready-for-review"
	StatusDone            Status = "done"
)

// ValidStatus reports whether s is a known ticket status.
func ValidStatus(s Status) bool {
	switch s {
	case StatusOpen, StatusWorking, StatusBlocked, StatusReadyForReview, StatusDone:
		return true
	}
	return false
}

// EntryKind classifies a thread entry.
type EntryKind string

const (
	EntryKindUpdate   EntryKind = "update"
	EntryKindQuestion EntryKind = "question"
	EntryKindAnswer   EntryKind = "answer"
	EntryKindComment  EntryKind = "comment"
	EntryKindImage    EntryKind = "image"
)

// ThreadEntry is a single append-only entry in a ticket's thread.
// Stored as a JSON file under .tickets/threads/<ticket-id>/.
type ThreadEntry struct {
	ID         string    `json:"id"`
	Kind       EntryKind `json:"kind"`
	Author     string    `json:"author"`
	Timestamp  time.Time `json:"ts"`
	Text       string    `json:"text,omitempty"`
	QuestionID string    `json:"question_id,omitempty"` // for answer entries
	ImageSHA   string    `json:"image_sha,omitempty"`   // for image entries
	ImageMIME  string    `json:"image_mime,omitempty"`  // for image entries
	Caption    string    `json:"caption,omitempty"`     // for image entries
}

// MarshalEntry serializes a thread entry to JSON bytes.
func MarshalEntry(e *ThreadEntry) ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEntry deserializes a thread entry from JSON bytes.
func UnmarshalEntry(data []byte) (*ThreadEntry, error) {
	var e ThreadEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("ticket: unmarshal entry: %w", err)
	}
	return &e, nil
}

// Ticket is the core data type stored in .tickets/ file storage.
type Ticket struct {
	ID          string    `yaml:"id"`
	Desc        string    `yaml:"desc"`
	Status      Status    `yaml:"status"`
	Agent       string    `yaml:"agent,omitempty"`
	Parent      string    `yaml:"parent,omitempty"`
	BlockReason string    `yaml:"block_reason,omitempty"`
	CreatedAt   time.Time `yaml:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at"`
}

// NewID generates a random 12-hex-char ticket ID.
func NewID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("ticket: generate id: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// NewQuestionID generates a random question ID of the form "q-XXXXXX".
func NewQuestionID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("ticket: generate question id: %w", err)
	}
	return fmt.Sprintf("q-%x", b), nil
}

// NewEntryID generates a random entry ID of the form "e-XXXXXX".
func NewEntryID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("ticket: generate entry id: %w", err)
	}
	return fmt.Sprintf("e-%x", b), nil
}

// Marshal serializes a ticket to YAML bytes.
func Marshal(t *Ticket) ([]byte, error) {
	return yaml.Marshal(t)
}

// Unmarshal deserializes a ticket from YAML bytes.
func Unmarshal(data []byte) (*Ticket, error) {
	var t Ticket
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("ticket: unmarshal: %w", err)
	}
	return &t, nil
}
