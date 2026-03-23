package ui

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// handleSSE streams new lines from events.log as Server-Sent Events.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Open events.log, seek to end so we only stream new events.
	logPath := s.loreDir + "/events.log"
	f, err := os.Open(logPath)
	if err != nil {
		// File may not exist yet; wait for it.
		fmt.Fprintf(w, ": waiting for events\n\n")
		flusher.Flush()
	}

	var offset int64
	if f != nil {
		offset, _ = f.Seek(0, io.SeekEnd)
		f.Close()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f, err := os.Open(logPath)
			if err != nil {
				continue
			}
			f.Seek(offset, io.SeekStart) //nolint
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", line)
				offset += int64(len(line)) + 1 // +1 for newline
			}
			f.Close()
			flusher.Flush()
		}
	}
}
