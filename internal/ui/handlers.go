package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/loreteam/lore/internal/ticketops"
)

func (s *Server) base(page string) BaseData {
	return BaseData{Page: page, RepoName: s.repoName}
}

// handleDashboard renders the review dashboard.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tickets, err := ticketops.ListTickets(ctx, s.gitRoot, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := DashboardData{BaseData: s.base("dashboard")}
	for _, t := range tickets {
		entries, _ := ticketops.LoadThread(ctx, s.gitRoot, t)
		view := TicketView{
			Ticket: t,
			Thread: entries,
			Age:    relativeTime(t.UpdatedAt),
		}
		switch t.Status {
		case ticket.StatusReadyForReview:
			data.ReadyForReview = append(data.ReadyForReview, view)
		case ticket.StatusBlocked:
			data.Blocked = append(data.Blocked, view)
		default:
			qs := openQuestions(entries)
			if len(qs) > 0 {
				view.OpenQuestions = qs
				data.NeedsAnswer = append(data.NeedsAnswer, view)
			}
		}
	}
	sortTicketViews(data.NeedsAnswer)
	sortTicketViews(data.ReadyForReview)
	sortTicketViews(data.Blocked)
	s.render(w, "dashboard", data)
}

// handleTicketList renders the full ticket list with optional status filter.
func (s *Server) handleTicketList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := r.URL.Query().Get("status")
	includeDone := filter == "done" || filter == ""

	tickets, err := ticketops.ListTickets(ctx, s.gitRoot, includeDone)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := TicketsData{BaseData: s.base("tickets"), StatusFilter: filter}
	for _, t := range tickets {
		if filter != "" && string(t.Status) != filter {
			continue
		}
		data.Tickets = append(data.Tickets, TicketView{
			Ticket: t,
			Age:    relativeTime(t.UpdatedAt),
		})
	}
	sortTicketViews(data.Tickets)
	s.render(w, "tickets", data)
}

// handleTicketDetail renders a single ticket with its full thread.
func (s *Server) handleTicketDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	entries, err := ticketops.LoadThread(ctx, s.gitRoot, t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.render(w, "detail", DetailData{
		BaseData: s.base("detail"),
		Ticket:   t,
		Thread:   entries,
		Age:      relativeTime(t.CreatedAt),
	})
}

// handleNewTicketForm renders the new ticket creation form.
func (s *Server) handleNewTicketForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "new", NewTicketData{BaseData: s.base("new")})
}

// handleNewTicketSubmit creates a new ticket from form data.
func (s *Server) handleNewTicketSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		r.ParseForm() //nolint
	}

	desc := strings.TrimSpace(r.FormValue("desc"))
	if desc == "" {
		s.render(w, "new", NewTicketData{BaseData: s.base("new"), Error: "description is required"})
		return
	}

	id, err := ticket.NewID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	t := &ticket.Ticket{
		ID:        id,
		Desc:      desc,
		Status:    ticket.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := ticketops.SaveTicket(ctx, s.gitRoot, t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Attach image if provided.
	if f, fh, err := r.FormFile("image"); err == nil {
		defer f.Close()
		imgData, err := io.ReadAll(f)
		if err == nil {
			s.attachImage(ctx, t, imgData, fh.Header.Get("Content-Type"), "")
		}
	}

	s.emitter.Emit(ctx, event.New(event.EventTicketCreated, map[string]any{"ticket_id": id})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleApprove approves a ready-for-review ticket.
func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	from := s.resolveAgent(ctx)

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if t.Status != ticket.StatusReadyForReview {
		http.Error(w, "ticket is not ready-for-review", http.StatusBadRequest)
		return
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      "approved",
	}
	if _, err := ticketops.AppendThread(ctx, s.gitRoot, t, entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitter.Emit(ctx, event.New(event.EventTicketApproved, map[string]any{"ticket_id": id, "from": from})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleReject rejects a ready-for-review ticket with a reason.
func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	from := s.resolveAgent(ctx)
	r.ParseForm() //nolint
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      fmt.Sprintf("rejected: %s", reason),
	}
	if _, err := ticketops.AppendThread(ctx, s.gitRoot, t, entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := ticketops.CASUpdate(ctx, s.gitRoot, id, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusWorking
		return nil
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitter.Emit(ctx, event.New(event.EventTicketRejected, map[string]any{"ticket_id": id, "from": from, "reason": reason})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleUnblock clears a block with an optional note.
func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	from := s.resolveAgent(ctx)
	r.ParseForm() //nolint
	note := strings.TrimSpace(r.FormValue("note"))

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	text := "unblocked"
	if note != "" {
		text = fmt.Sprintf("unblocked: %s", note)
	}
	entryID, err := ticket.NewEntryID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindComment,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      text,
	}
	if _, err := ticketops.AppendThread(ctx, s.gitRoot, t, entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := ticketops.CASUpdate(ctx, s.gitRoot, id, func(t *ticket.Ticket) error {
		t.Status = ticket.StatusWorking
		t.BlockReason = ""
		return nil
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitter.Emit(ctx, event.New(event.EventTicketUnblocked, map[string]any{"ticket_id": id, "from": from})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleAnswer posts an answer to a question on a ticket.
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	from := s.resolveAgent(ctx)
	r.ParseForm() //nolint

	qid := r.FormValue("qid")
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		http.Error(w, "answer text is required", http.StatusBadRequest)
		return
	}

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &ticket.ThreadEntry{
		ID:         entryID,
		Kind:       ticket.EntryKindAnswer,
		Author:     from,
		Timestamp:  time.Now().UTC(),
		Text:       text,
		QuestionID: qid,
	}
	if _, err := ticketops.AppendThread(ctx, s.gitRoot, t, entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitter.Emit(ctx, event.New(event.EventQuestionAnswered, map[string]any{"ticket_id": id, "question_id": qid})) //nolint

	if isHTMX(r) {
		// Return a small "answered" confirmation snippet for the dashboard card.
		fmt.Fprintf(w, `<div class="rounded-md border border-green-200 dark:border-green-900 bg-green-50 dark:bg-green-950/40 px-4 py-3 text-sm text-green-700 dark:text-green-400">Question answered.</div>`)
		return
	}
	redirect(w, r, "/tickets/"+id)
}

// handleUpdate adds a free-form update or comment to a ticket.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	from := s.resolveAgent(ctx)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		r.ParseForm() //nolint
	}

	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindUpdate,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      text,
	}
	if _, err := ticketops.AppendThread(ctx, s.gitRoot, t, entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.emitter.Emit(ctx, event.New(event.EventTicketUpdated, map[string]any{"ticket_id": id})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleImage attaches an image upload to a ticket's thread.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}

	f, fh, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image is required", http.StatusBadRequest)
		return
	}
	defer f.Close()

	imgData, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "could not read image", http.StatusInternalServerError)
		return
	}

	t, err := ticketops.LoadTicket(ctx, s.gitRoot, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	mime := fh.Header.Get("Content-Type")
	caption := strings.TrimSpace(r.FormValue("caption"))
	s.attachImage(ctx, t, imgData, mime, caption)
	s.emitter.Emit(ctx, event.New(event.EventTicketUpdated, map[string]any{"ticket_id": id, "kind": "image"})) //nolint
	redirect(w, r, "/tickets/"+id)
}

// handleSearch renders search results for a query.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	data := SearchData{BaseData: s.base("search"), Query: q}

	if q != "" {
		tickets, _ := ticketops.ListTickets(ctx, s.gitRoot, true)
		qLower := strings.ToLower(q)
		for _, t := range tickets {
			if strings.Contains(strings.ToLower(t.Desc), qLower) ||
				strings.Contains(strings.ToLower(t.ID), qLower) {
				data.Results = append(data.Results, SearchResult{
					Ticket:  t,
					Snippet: snippet(t.Desc, q, 120),
					Age:     relativeTime(t.UpdatedAt),
				})
			}
		}
	}

	s.render(w, "search", data)
}

// handleEventLog renders the event log page (the SSE stream is a separate endpoint).
func (s *Server) handleEventLog(w http.ResponseWriter, r *http.Request) {
	s.render(w, "events", EventsData{BaseData: s.base("events")})
}

// attachImage writes the image bytes as a git blob and appends an image thread entry.
// Consistent with the CLI: the blob SHA is stored in ImageSHA and served via /blobs/{sha}.
func (s *Server) attachImage(ctx context.Context, t *ticket.Ticket, data []byte, mime, caption string) {
	from := s.resolveAgent(ctx)
	imgSHA, err := gitcmd.WriteBlob(ctx, s.gitRoot, data)
	if err != nil {
		return
	}
	entryID, err := ticket.NewEntryID()
	if err != nil {
		return
	}
	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindImage,
		Author:    from,
		Timestamp: time.Now().UTC(),
		ImageSHA:  imgSHA,
		ImageMIME: mime,
		Caption:   caption,
	}
	ticketops.AppendThread(ctx, s.gitRoot, t, entry) //nolint
}

// handleBlob serves a raw git blob by SHA. Used to render images stored by any path (CLI or UI).
func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha")
	data, err := gitcmd.ReadBlob(r.Context(), s.gitRoot, sha)
	if err != nil {
		http.Error(w, "blob not found", http.StatusNotFound)
		return
	}
	ct := http.DetectContentType(data)
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(data) //nolint
}

// resolveAgent returns the UI agent identifier.
func (s *Server) resolveAgent(_ context.Context) string {
	return "human"
}

// snippet returns up to maxLen chars of text centred around the first occurrence of query.
func snippet(text, query string, maxLen int) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		if utf8.RuneCountInString(text) <= maxLen {
			return text
		}
		return string([]rune(text)[:maxLen]) + "…"
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	runes := []rune(text)
	if start >= len(runes) {
		start = 0
	}
	end := start + maxLen
	if end > len(runes) {
		end = len(runes)
	}
	s := string(runes[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(runes) {
		s = s + "…"
	}
	return s
}
