package problem

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// ctxWithSpan returns a context carrying a valid, non-recording span context,
// which is enough to exercise trace ID propagation without an SDK.
func ctxWithSpan(t *testing.T) (context.Context, string) {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("span id: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc), traceID.String()
}

func TestWriteSetsMediaTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(context.Background(), rec, NotFound("no such widget").WithInstance("/widgets/42"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != MediaType {
		t.Errorf("Content-Type = %q, want %q", ct, MediaType)
	}
	if v := rec.Header().Get("X-Content-Type-Options"); v != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", v)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body)
	}
	if body["status"] != float64(404) || body["instance"] != "/widgets/42" {
		t.Errorf("body = %v", body)
	}
}

func TestWriteAddsTraceIDExtension(t *testing.T) {
	ctx, wantTraceID := ctxWithSpan(t)
	rec := httptest.NewRecorder()

	Write(ctx, rec, Internal(errors.New("boom")))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body[ExtTraceID] != wantTraceID {
		t.Errorf("%s = %v, want %s", ExtTraceID, body[ExtTraceID], wantTraceID)
	}
}

func TestWriteOmitsTraceIDWithoutSpan(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(context.Background(), rec, NotFound("gone"))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, present := body[ExtTraceID]; present {
		t.Errorf("%s should be absent without a valid span context", ExtTraceID)
	}
}

func TestWriteSendsProblemHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(context.Background(), rec, Unavailable("Service is starting up.", 5))

	if v := rec.Header().Get("Retry-After"); v != "5" {
		t.Errorf("Retry-After = %q, want 5", v)
	}
}

func TestWriteDoesNotLeakCause(t *testing.T) {
	rec := httptest.NewRecorder()
	secret := "dial tcp 10.0.0.7:5432: connection refused"

	Write(context.Background(), rec, Internal(errors.New(secret)))

	if strings.Contains(rec.Body.String(), "10.0.0.7") {
		t.Fatalf("cause leaked into response: %s", rec.Body)
	}
}

func TestWriteNilProblemIsInternalError(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(context.Background(), rec, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != MediaType {
		t.Errorf("Content-Type = %q, want %q", ct, MediaType)
	}
}

func TestWriteRecordsProblemTypeForMetrics(t *testing.T) {
	ctx, recorder := ContextWithRecorder(context.Background())
	rec := httptest.NewRecorder()

	Write(ctx, rec, Unauthorized("nope").WithType(TypeInvalidToken))

	if got := recorder.Type(); got != TypeInvalidToken {
		t.Errorf("Recorder.Type() = %q, want %q", got, TypeInvalidToken)
	}
}

// The first problem written wins, so an outer handler cannot mask the cause.
func TestRecorderKeepsFirstProblemType(t *testing.T) {
	ctx, recorder := ContextWithRecorder(context.Background())

	Write(ctx, httptest.NewRecorder(), Unauthorized("nope").WithType(TypeInvalidToken))
	Write(ctx, httptest.NewRecorder(), Internal(errors.New("later")))

	if got := recorder.Type(); got != TypeInvalidToken {
		t.Errorf("Recorder.Type() = %q, want the first problem type", got)
	}
}

func TestRecorderEmptyWhenNoProblemWritten(t *testing.T) {
	_, recorder := ContextWithRecorder(context.Background())
	if got := recorder.Type(); got != "" {
		t.Errorf("Recorder.Type() = %q, want empty", got)
	}
}

func TestWriteErrorUnwrapsProblem(t *testing.T) {
	p := Forbidden("not your widget").WithType("https://example.com/forbidden")
	wrapped := errors.Join(errors.New("context"), p)

	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, wrapped)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["type"] != "https://example.com/forbidden" {
		t.Errorf("type = %v, want the wrapped problem's type", body["type"])
	}
}

func TestWriteErrorFallsBackToInternal(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteError(context.Background(), rec, errors.New("some plain error"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "some plain error") {
		t.Fatalf("plain error leaked into response: %s", rec.Body)
	}
}
