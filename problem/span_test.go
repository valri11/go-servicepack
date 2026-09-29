package problem

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func recordingCtx(t *testing.T) (context.Context, func() sdktrace.ReadOnlySpan) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	ctx, span := tp.Tracer("test").Start(context.Background(), "server")
	return ctx, func() sdktrace.ReadOnlySpan {
		span.End()
		ended := sr.Ended()
		if len(ended) != 1 {
			t.Fatalf("ended spans = %d, want 1", len(ended))
		}
		return ended[0]
	}
}

func spanAttr(s sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestWriteLeaves4xxSpanStatusUnset(t *testing.T) {
	ctx, end := recordingCtx(t)

	Write(ctx, httptest.NewRecorder(), InvalidParams(InvalidParam{Name: "age", Reason: "bad"}))

	s := end()
	if s.Status().Code != codes.Unset {
		t.Errorf("status = %v, want Unset", s.Status().Code)
	}
	if v, _ := spanAttr(s, "problem.type"); v.AsString() != TypeValidationError {
		t.Errorf("problem.type = %q, want %q", v.AsString(), TypeValidationError)
	}
	if v, _ := spanAttr(s, "problem.status"); v.AsInt64() != http.StatusBadRequest {
		t.Errorf("problem.status = %d, want 400", v.AsInt64())
	}
}

func TestWriteMarks5xxSpanError(t *testing.T) {
	ctx, end := recordingCtx(t)

	Write(ctx, httptest.NewRecorder(), Internal(errors.New("db down")))

	s := end()
	if s.Status().Code != codes.Error {
		t.Errorf("status = %v, want Error", s.Status().Code)
	}
	if len(s.Events()) == 0 || s.Events()[0].Name != "exception" {
		t.Errorf("cause not recorded as exception event: %v", s.Events())
	}
}

func TestRecovererAbortsWhenResponseStarted(t *testing.T) {
	ctx, end := recordingCtx(t)
	h := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late failure")
	}))

	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if r := recover(); r != http.ErrAbortHandler {
				t.Errorf("recovered %v, want http.ErrAbortHandler", r)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	}()

	if got := rec.Body.String(); got != "partial" {
		t.Errorf("body = %q, want only the partial response", got)
	}
	s := end()
	if s.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", s.Status().Code)
	}
	if v, _ := spanAttr(s, "problem.type"); v.AsString() != TypePanic {
		t.Errorf("problem.type = %q, want %q", v.AsString(), TypePanic)
	}
}
