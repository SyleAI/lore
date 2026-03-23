package ticket

import (
	"crypto/rand"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Status represents the lifecycle state of a ticket.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in-progress"
	StatusBlocked    Status = "blocked"
	StatusReady      Status = "ready"
	StatusClosed     Status = "closed"
)

// QuestionType classifies the kind of question an agent is asking.
type QuestionType string

const (
	QuestionTypeClarification   QuestionType = "clarification"
	QuestionTypeConstraintCheck QuestionType = "constraint_check"
	QuestionTypeKnowledgeGap    QuestionType = "knowledge_gap"
	QuestionTypeValidation      QuestionType = "validation"
)

// ValidQuestionType reports whether qt is a known question type.
func ValidQuestionType(qt QuestionType) bool {
	switch qt {
	case QuestionTypeClarification, QuestionTypeConstraintCheck,
		QuestionTypeKnowledgeGap, QuestionTypeValidation:
		return true
	}
	return false
}

// Checkpoint is a timestamped progress note recorded by an agent.
type Checkpoint struct {
	Timestamp time.Time `yaml:"ts"`
	Message   string    `yaml:"msg"`
}

// Question is an async question an agent raises on a ticket.
type Question struct {
	ID         string       `yaml:"id"`
	Type       QuestionType `yaml:"type"`
	Text       string       `yaml:"text"`
	Blocking   bool         `yaml:"blocking"`
	Assumption string       `yaml:"assumption,omitempty"`
	DirectedTo string       `yaml:"directed_to,omitempty"`
	Answer     string       `yaml:"answer,omitempty"`
	AnsweredBy string       `yaml:"answered_by,omitempty"`
	AskedAt    time.Time    `yaml:"asked_at"`
	AnsweredAt *time.Time   `yaml:"answered_at,omitempty"`
}

// Ticket is the core data type stored in git object storage.
type Ticket struct {
	ID          string         `yaml:"id"`
	Title       string         `yaml:"title"`
	Description string         `yaml:"description,omitempty"`
	Status      Status         `yaml:"status"`
	Priority    int            `yaml:"priority"`
	Files       []string       `yaml:"files,omitempty"`
	Agent       string         `yaml:"agent,omitempty"`
	Parent      string         `yaml:"parent,omitempty"`
	BlockReason string         `yaml:"block_reason,omitempty"`
	Attempts    int            `yaml:"attempts,omitempty"`
	Checkpoints []Checkpoint   `yaml:"checkpoints,omitempty"`
	Questions   []Question     `yaml:"questions,omitempty"`
	Metadata    map[string]any `yaml:"metadata,omitempty"`
	CreatedAt   time.Time      `yaml:"created_at"`
	UpdatedAt   time.Time      `yaml:"updated_at"`
}

// ValidStatus reports whether s is a known ticket status.
func ValidStatus(s Status) bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusBlocked, StatusReady, StatusClosed:
		return true
	}
	return false
}

// Ref returns the git ref path for a ticket ID.
func Ref(id string) string {
	return "refs/tickets/t/" + id
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
