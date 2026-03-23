package event

import "time"

// EventType identifies the kind of event that occurred.
type EventType string

const (
	EventTicketCreated   EventType = "ticket.created"
	EventTicketAssigned  EventType = "ticket.assigned"
	EventTicketClaimed   EventType = "ticket.claimed"
	EventTicketBlocked   EventType = "ticket.blocked"
	EventTicketReady     EventType = "ticket.ready"
	EventTicketApproved  EventType = "ticket.approved"
	EventTicketRejected  EventType = "ticket.rejected"
	EventTicketUpdated   EventType = "ticket.updated"
	EventTicketClosed    EventType = "ticket.closed"
	EventTicketUnblocked EventType = "ticket.unblocked"
	EventQuestionAsked   EventType = "question.asked"
	EventQuestionAnswered EventType = "question.answered"
	EventQuestionTagged  EventType = "question.tagged"
	EventLoreInitialized EventType = "lore.initialized"
)

// Event is an immutable record of something that happened in the system.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"ts"`
	Data      map[string]any `json:"data,omitempty"`
}

// New creates a new Event with the given type and data, stamping the current UTC time.
func New(t EventType, data map[string]any) Event {
	return Event{
		Type:      t,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}
