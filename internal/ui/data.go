package ui

import (
	"sort"

	"github.com/loreteam/lore/internal/ticket"
)

// BaseData is embedded in every page's data struct.
type BaseData struct {
	Page     string // "dashboard", "tickets", "detail", "new", "search", "events"
	RepoName string
}

// TicketView wraps a ticket with precomputed display fields.
type TicketView struct {
	*ticket.Ticket
	Thread        []*ticket.ThreadEntry
	Age           string
	OpenQuestions []*ticket.ThreadEntry
}

// openQuestions returns all unanswered question entries in the thread.
func openQuestions(entries []*ticket.ThreadEntry) []*ticket.ThreadEntry {
	// Build a set of answered question IDs.
	answered := map[string]bool{}
	for _, e := range entries {
		if e.Kind == ticket.EntryKindAnswer && e.QuestionID != "" {
			answered[e.QuestionID] = true
		}
	}
	var qs []*ticket.ThreadEntry
	for _, e := range entries {
		if e.Kind == ticket.EntryKindQuestion && !answered[e.QuestionID] {
			qs = append(qs, e)
		}
	}
	return qs
}

// DashboardData is passed to the dashboard template.
type DashboardData struct {
	BaseData
	NeedsAnswer    []TicketView
	ReadyForReview []TicketView
	Blocked        []TicketView
}

// TicketsData is passed to the ticket list template.
type TicketsData struct {
	BaseData
	Tickets      []TicketView
	StatusFilter string
}

// DetailData is passed to the ticket detail template.
type DetailData struct {
	BaseData
	Ticket *ticket.Ticket
	Thread []*ticket.ThreadEntry
	Age    string
}

// NewTicketData is passed to the new ticket form template.
type NewTicketData struct {
	BaseData
	Error string
}

// SearchData is passed to the search template.
type SearchData struct {
	BaseData
	Query   string
	Results []SearchResult
}

// SearchResult is a single search hit.
type SearchResult struct {
	*ticket.Ticket
	Snippet string
	Age     string
}

// EventsData is passed to the event log template.
type EventsData struct {
	BaseData
}

// sortTicketViews sorts by UpdatedAt descending (most recently updated first).
func sortTicketViews(views []TicketView) {
	sort.Slice(views, func(i, j int) bool {
		return views[i].UpdatedAt.After(views[j].UpdatedAt)
	})
}
