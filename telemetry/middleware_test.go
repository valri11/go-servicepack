package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const (
	incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	incomingSpanID  = "00f067aa0ba902b7"
	traceparent     = "00-" + incomingTraceID + "-" + incomingSpanID + "-01"
)

func newTestTracing(t *testing.T) (*tracetest.SpanRecorder, []otelhttp.Option) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	return sr, []otelhttp.Option{
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithPropagators(propagation.TraceContext{}),
	}
}

func TestHTTPMiddlewareJoinsIncomingTrace(t *testing.T) {
	sr, opts := newTestTracing(t)

	var handlerSpan trace.SpanContext
	route := HTTPMiddleware(opts...)(WithRequestLog()(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			handlerSpan = trace.SpanContextFromContext(r.Context())
		})))

	mux := http.NewServeMux()
	mux.Handle("GET /items/{id}", route)

	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	req.Header.Set("traceparent", traceparent)
	mux.ServeHTTP(httptest.NewRecorder(), req)

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want only the SERVER span", len(spans))
	}
	s := spans[0]
	if s.SpanKind() != trace.SpanKindServer {
		t.Errorf("kind = %v, want server", s.SpanKind())
	}
	if got := s.SpanContext().TraceID().String(); got != incomingTraceID {
		t.Errorf("trace id = %s, want incoming %s", got, incomingTraceID)
	}
	if got := s.Parent().SpanID().String(); got != incomingSpanID || !s.Parent().IsRemote() {
		t.Errorf("parent = %s (remote %v), want remote %s", got, s.Parent().IsRemote(), incomingSpanID)
	}
	if s.Name() != "GET /items/{id}" {
		t.Errorf("name = %q, want %q", s.Name(), "GET /items/{id}")
	}
	if handlerSpan.SpanID() != s.SpanContext().SpanID() {
		t.Errorf("handler ran outside the SERVER span")
	}
}

func TestRoute(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"/":                      "/",
		"/items/{id}":            "/items/{id}",
		"GET /livez":             "/livez",
		"GET example.com/livez":  "/livez",
		"example.com/items/{id}": "/items/{id}",
	}
	for pattern, want := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Pattern = pattern
		if got := Route(r); got != want {
			t.Errorf("Route(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestSpanNameWithoutRouteIsMethodOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/random/path/123", nil)
	if got := spanName("", r); got != http.MethodPost {
		t.Errorf("span name = %q, want %q", got, http.MethodPost)
	}
}
