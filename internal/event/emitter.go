package event

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Emitter can publish an event to some destination.
type Emitter interface {
	Emit(ctx context.Context, e Event) error
}

type contextKey struct{}

// EmitterKey is the key used to store/retrieve an Emitter from a context.
var EmitterKey = contextKey{}

// EmitterFromContext returns the Emitter stored in ctx.
// If no Emitter is present, it returns a NoopEmitter.
func EmitterFromContext(ctx context.Context) Emitter {
	if e, ok := ctx.Value(EmitterKey).(Emitter); ok && e != nil {
		return e
	}
	return NewNoopEmitter()
}

// fanoutEmitter distributes events to all adapters concurrently.
type fanoutEmitter struct {
	adapters []Emitter
}

// NewFanoutEmitter returns an Emitter that fans out to all provided adapters concurrently.
// Each adapter is given a 5-second timeout. Failures are logged to stderr but never
// propagated to the caller.
func NewFanoutEmitter(adapters ...Emitter) Emitter {
	return &fanoutEmitter{adapters: adapters}
}

func (f *fanoutEmitter) Emit(ctx context.Context, e Event) error {
	var wg sync.WaitGroup
	for _, adapter := range f.adapters {
		wg.Add(1)
		go func(a Emitter) {
			defer wg.Done()
			adapterCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := a.Emit(adapterCtx, e); err != nil {
				fmt.Fprintf(stderr, "event: fanout emit error: %v\n", err)
			}
		}(adapter)
	}
	wg.Wait()
	return nil
}

// noopEmitter discards all events.
type noopEmitter struct{}

// NewNoopEmitter returns an Emitter that discards all events.
func NewNoopEmitter() Emitter {
	return &noopEmitter{}
}

func (n *noopEmitter) Emit(_ context.Context, _ Event) error {
	return nil
}
