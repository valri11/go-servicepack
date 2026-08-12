package problem

import (
	"context"
	"sync"
)

type ctxRecorderKey struct{}

// Recorder captures the problem type written during a request so middleware
// running outside the handler can label telemetry by cause.
//
// It travels in the context rather than on the ResponseWriter because
// intermediate middleware (otelhttp, compression) replace the writer with
// wrappers a type assertion would not see through.
type Recorder struct {
	mu       sync.Mutex
	problem  string
	recorded bool
}

func ContextWithRecorder(ctx context.Context) (context.Context, *Recorder) {
	rec := &Recorder{}
	return context.WithValue(ctx, ctxRecorderKey{}, rec), rec
}

func RecorderFromContext(ctx context.Context) (*Recorder, bool) {
	rec, ok := ctx.Value(ctxRecorderKey{}).(*Recorder)
	return rec, ok
}

// Type returns the recorded problem type, or "" if no problem was written.
func (r *Recorder) Type() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.problem
}

// record keeps the first problem type, so an outer handler cannot mask the
// original cause.
func (r *Recorder) record(typeURI string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recorded {
		return
	}
	r.recorded = true
	r.problem = typeURI
}
