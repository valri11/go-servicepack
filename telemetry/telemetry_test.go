package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestResolveEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		env      map[string]string
		want     string
	}{
		{name: "nothing set uses plaintext localhost", want: defaultEndpoint},
		{name: "explicit bare host is plaintext", explicit: "collector:4317", want: "http://collector:4317"},
		{name: "explicit https kept for TLS", explicit: "https://collector:4317", want: "https://collector:4317"},
		{
			name:     "explicit wins over env",
			explicit: "http://flag:4317",
			env:      map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://env:4317"},
			want:     "http://flag:4317",
		},
		{
			name: "env URL left to the SDK",
			env:  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://env:4317"},
			want: "",
		},
		{
			name: "env bare host normalized",
			env:  map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "10.0.0.1:4317"},
			want: "http://10.0.0.1:4317",
		},
		{
			name: "per-signal env left to the SDK",
			env:  map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://tempo:4317"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
			for _, k := range signalEndpointEnv {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := resolveEndpoint(tt.explicit); got != tt.want {
				t.Errorf("resolveEndpoint(%q) = %q, want %q", tt.explicit, got, tt.want)
			}
		})
	}
}

func TestExporters(t *testing.T) {
	tests := []struct {
		val           string
		otlp, console bool
	}{
		{"", true, false},
		{"otlp", true, false},
		{"console", false, true},
		{"otlp, console", true, true},
		{"none", false, false},
	}
	for _, tt := range tests {
		t.Setenv("OTEL_TRACES_EXPORTER", tt.val)
		otlp, console := exporters("OTEL_TRACES_EXPORTER")
		if otlp != tt.otlp || console != tt.console {
			t.Errorf("exporters(%q) = %v, %v; want %v, %v", tt.val, otlp, console, tt.otlp, tt.console)
		}
	}
}

func TestTraceContextHandlerAddsIDs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(traceContextHandler{slog.NewJSONHandler(&buf, nil)})

	traceID, _ := trace.TraceIDFromHex(incomingTraceID)
	spanID, _ := trace.SpanIDFromHex(incomingSpanID)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))

	logger.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log is not JSON: %v", err)
	}
	if rec["trace_id"] != incomingTraceID || rec["span_id"] != incomingSpanID {
		t.Errorf("record = %v, want trace_id and span_id", rec)
	}

	buf.Reset()
	logger.Info("no span")
	if bytes.Contains(buf.Bytes(), []byte("trace_id")) {
		t.Errorf("trace_id added without a span: %s", buf.String())
	}
}

func TestLevelHandler(t *testing.T) {
	var buf bytes.Buffer
	h := levelHandler{
		level:   slog.LevelWarn,
		Handler: slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
	logger := slog.New(h)

	logger.Info("dropped")
	logger.Warn("kept")

	if bytes.Contains(buf.Bytes(), []byte("dropped")) || !bytes.Contains(buf.Bytes(), []byte("kept")) {
		t.Errorf("output = %s", buf.String())
	}
}
