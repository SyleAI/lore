package event

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// fileEmitter appends newline-delimited JSON events to a file.
type fileEmitter struct {
	path string
	mu   sync.Mutex
}

// NewFileEmitter returns an Emitter that appends NDJSON events to path.
// The file is opened in append mode on each write (so log rotation is safe).
func NewFileEmitter(path string) Emitter {
	return &fileEmitter{path: path}
}

func (f *fileEmitter) Emit(_ context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("event: marshal: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("event: open log: %w", err)
	}
	defer fh.Close()

	_, err = fmt.Fprintf(fh, "%s\n", data)
	return err
}
