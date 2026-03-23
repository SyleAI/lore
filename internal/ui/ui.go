// Package ui provides the local web interface for Lore.
package ui

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/ticket"
)

//go:embed templates
var templateFS embed.FS

// Server is the Lore web UI HTTP server.
type Server struct {
	gitRoot  string
	loreDir  string
	port     int
	repoName string
	emitter  event.Emitter
	funcMap  template.FuncMap
}

// NewServer creates a new UI server for the given git repository.
func NewServer(gitRoot string, port int) *Server {
	repoName := filepath.Base(gitRoot)
	loreDir := filepath.Join(gitRoot, ".lore")
	s := &Server{
		gitRoot:  gitRoot,
		loreDir:  loreDir,
		port:     port,
		repoName: repoName,
		emitter:  event.NewFileEmitter(filepath.Join(loreDir, "events.log")),
	}
	s.funcMap = template.FuncMap{
		"relTime":        relativeTime,
		"statusBadge":    statusBadgeClass,
		"statusLabel":    statusLabel,
		"isQuestion":     func(e *ticket.ThreadEntry) bool { return e.Kind == ticket.EntryKindQuestion },
		"isAnswer":       func(e *ticket.ThreadEntry) bool { return e.Kind == ticket.EntryKindAnswer },
		"isImage":        func(e *ticket.ThreadEntry) bool { return e.Kind == ticket.EntryKindImage },
		"isReadyReview":  func(t *ticket.Ticket) bool { return t.Status == ticket.StatusReadyForReview },
		"isBlocked":      func(t *ticket.Ticket) bool { return t.Status == ticket.StatusBlocked },
		"hasPrefix":      strings.HasPrefix,
	}
	return s
}

// Start registers routes and begins serving on localhost:port.
// It blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /tickets", s.handleTicketList)
	mux.HandleFunc("GET /tickets/new", s.handleNewTicketForm)
	mux.HandleFunc("POST /tickets/new", s.handleNewTicketSubmit)
	mux.HandleFunc("GET /tickets/{id}", s.handleTicketDetail)
	mux.HandleFunc("POST /tickets/{id}/approve", s.handleApprove)
	mux.HandleFunc("POST /tickets/{id}/reject", s.handleReject)
	mux.HandleFunc("POST /tickets/{id}/unblock", s.handleUnblock)
	mux.HandleFunc("POST /tickets/{id}/answer", s.handleAnswer)
	mux.HandleFunc("POST /tickets/{id}/update", s.handleUpdate)
	mux.HandleFunc("POST /tickets/{id}/image", s.handleImage)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /events", s.handleEventLog)
	mux.HandleFunc("GET /events/stream", s.handleSSE)
	mux.HandleFunc("GET /blobs/{sha}", s.handleBlob)

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("lore ui: listen %s: %w", addr, err)
	}

	srv := &http.Server{Handler: mux}
	fmt.Printf("Lore UI running at http://%s\n", addr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// render executes the named page template (base + page) writing to w.
func (s *Server) render(w http.ResponseWriter, page string, data any) {
	tmpl, err := template.New("").Funcs(s.funcMap).ParseFS(
		templateFS,
		"templates/base.html",
		"templates/"+page+".html",
	)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		// Headers already sent, just log.
		fmt.Printf("ui: render %s: %v\n", page, err)
	}
}

// redirect sends a 303 See Other to the given path.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// isHTMX returns true if the request was made by HTMX.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// relativeTime formats a time as a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// statusBadgeClass returns Tailwind classes for a status badge.
func statusBadgeClass(s ticket.Status) string {
	switch s {
	case ticket.StatusOpen:
		return "bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-400"
	case ticket.StatusWorking:
		return "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-400"
	case ticket.StatusBlocked:
		return "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400"
	case ticket.StatusReadyForReview:
		return "bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-400"
	case ticket.StatusDone:
		return "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-400"
	default:
		return "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
	}
}

// statusLabel returns a display label for a status.
func statusLabel(s ticket.Status) string {
	switch s {
	case ticket.StatusReadyForReview:
		return "Ready"
	default:
		return string(s)
	}
}
