package problem

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const fallbackBody = `{"type":"about:blank","title":"Internal Server Error","status":500}`

// Write sends p as an RFC 9457 problem+json response, adds the trace ID as an
// extension member, records the cause on the active span, logs it, and reports
// the problem type to the context Recorder.
//
// It is the single exit point for error responses; handlers should not write
// error bodies directly.
func Write(ctx context.Context, w http.ResponseWriter, p *Problem) {
	if p == nil {
		p = Internal(errors.New("nil problem"))
	}
	p.normalize()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		p.With(ExtTraceID, sc.TraceID().String())
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		if p.cause != nil {
			span.RecordError(p.cause)
		}
		span.SetStatus(codes.Error, p.Title)
		span.SetAttributes(
			attribute.String("problem.type", p.Type),
			attribute.Int("problem.status", p.Status),
		)
	}

	if rec, ok := RecorderFromContext(ctx); ok {
		rec.record(p.Type)
	}

	logAttrs := []any{
		"status", p.Status,
		"problem_type", p.Type,
		"detail", p.Detail,
	}
	if p.Instance != "" {
		logAttrs = append(logAttrs, "instance", p.Instance)
	}
	if p.cause != nil {
		logAttrs = append(logAttrs, "error", p.cause)
	}
	if p.Status >= http.StatusInternalServerError {
		slog.ErrorContext(ctx, p.Title, logAttrs...)
	} else {
		slog.WarnContext(ctx, p.Title, logAttrs...)
	}

	body, err := json.Marshal(p)
	if err != nil {
		slog.ErrorContext(ctx, "problem: failed to marshal problem details", "error", err)
		body = []byte(fallbackBody)
		p.Status = http.StatusInternalServerError
	}

	header := w.Header()
	for k, v := range p.headers {
		header.Set(k, v)
	}
	header.Set("Content-Type", MediaType)
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(p.Status)

	if _, err := w.Write(body); err != nil {
		slog.WarnContext(ctx, "problem: failed to write response", "error", err)
	}
}

// WriteError sends err as a problem. If err is or wraps a *Problem that
// problem is used, otherwise err becomes a generic 500 with err as the cause.
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	var p *Problem
	if errors.As(err, &p) {
		Write(ctx, w, p)
		return
	}
	Write(ctx, w, Internal(err))
}
