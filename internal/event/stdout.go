package event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// stderr is used for error logging from the fanout emitter.
var stderr io.Writer = os.Stderr

// stdoutEmitter writes JSON events as newline-delimited JSON to stdout.
// It is thread-safe.
type stdoutEmitter struct {
	w   io.Writer
	mu  sync.Mutex
}

// NewStdoutEmitter returns an Emitter that writes newline-delimited JSON to stdout.
func NewStdoutEmitter() Emitter {
	return &stdoutEmitter{w: os.Stdout}
}

// NewWriterEmitter returns an Emitter that writes newline-delimited JSON to w.
// Useful for testing.
func NewWriterEmitter(w io.Writer) Emitter {
	return &stdoutEmitter{w: w}
}

func (s *stdoutEmitter) Emit(_ context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("event: marshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintf(s.w, "%s\n", data)
	return err
}
