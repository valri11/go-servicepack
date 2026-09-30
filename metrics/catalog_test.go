package metrics

import (
	"bufio"
	"context"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/valri11/go-servicepack/problem"
	"github.com/valri11/go-servicepack/telemetry"
)

const catalogPath = "../docs/metrics.md"

// TestTelemetryCatalog runs the library's instrumentation end to end and
// checks what it emits against docs/metrics.md: every metric must be
// documented and every documented metric emitted; every metric, span and log
// attribute key must be documented.
func TestTelemetryCatalog(t *testing.T) {
	emitted := emitTelemetry(t)
	doc := readCatalog(t)

	for _, name := range emitted.metrics {
		if !doc.metrics[name] {
			t.Errorf("metric %q is emitted but not in %s", name, catalogPath)
		}
	}
	for name := range doc.metrics {
		if !slices.Contains(emitted.metrics, name) {
			t.Errorf("metric %q is in %s but not emitted", name, catalogPath)
		}
	}
	for _, key := range emitted.attributes {
		if !doc.attributes[key] {
			t.Errorf("attribute %q is emitted but not in %s", key, catalogPath)
		}
	}
	for _, key := range emitted.logKeys {
		if !doc.logKeys[key] {
			t.Errorf("log key %q is emitted but not in %s", key, catalogPath)
		}
	}
}

type telemetrySet struct {
	metrics    []string
	attributes []string
	logKeys    []string
}

func emitTelemetry(t *testing.T) telemetrySet {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	logs := &keyRecorder{keys: map[string]bool{}}
	prev := slog.Default()
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		t.Fatal(err)
	}
	app, err := NewAppMetrics(mp.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}

	chain := func(h http.Handler) http.Handler {
		return telemetry.HTTPMiddleware(otelhttp.WithTracerProvider(tp), otelhttp.WithMeterProvider(mp))(
			telemetry.WithRequestLog()(WithMetrics(app)(problem.Recoverer(h))))
	}
	mux := http.NewServeMux()
	mux.Handle("GET /ok", chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})))
	mux.Handle("GET /invalid", chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		problem.Write(r.Context(), w, problem.InvalidParams(problem.InvalidParam{Name: "x", Reason: "bad"}).
			WithInstance(r.URL.Path))
	})))
	mux.Handle("GET /panic", chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	mux.Handle("/", chain(problem.NotFoundHandler()))

	for _, path := range []string{"/ok", "/invalid", "/panic", "/missing"} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	metricNames := map[string]bool{}
	attrKeys := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			metricNames[m.Name] = true
			for _, set := range dataPointAttributes(m.Data) {
				for _, kv := range set.ToSlice() {
					attrKeys[string(kv.Key)] = true
				}
			}
		}
	}
	for _, s := range spans.Ended() {
		for _, kv := range s.Attributes() {
			attrKeys[string(kv.Key)] = true
		}
		for _, e := range s.Events() {
			for _, kv := range e.Attributes {
				attrKeys[string(kv.Key)] = true
			}
		}
	}

	return telemetrySet{
		metrics:    slices.Sorted(maps.Keys(metricNames)),
		attributes: slices.Sorted(maps.Keys(attrKeys)),
		logKeys:    logs.sorted(),
	}
}

func dataPointAttributes(data metricdata.Aggregation) []attribute.Set {
	var sets []attribute.Set
	switch d := data.(type) {
	case metricdata.Sum[int64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	case metricdata.Gauge[int64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	case metricdata.Histogram[int64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, p := range d.DataPoints {
			sets = append(sets, p.Attributes)
		}
	}
	return sets
}

type catalog struct {
	metrics    map[string]bool
	attributes map[string]bool
	logKeys    map[string]bool
}

// readCatalog collects the backquoted first-column names of the tables under
// the "## Metrics", "## Attributes" and "## Log keys" headings.
func readCatalog(t *testing.T) catalog {
	t.Helper()
	f, err := os.Open(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	c := catalog{metrics: map[string]bool{}, attributes: map[string]bool{}, logKeys: map[string]bool{}}
	sections := map[string]map[string]bool{
		"## Metrics":    c.metrics,
		"## Attributes": c.attributes,
		"## Log keys":   c.logKeys,
	}
	var current map[string]bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			current = sections[line]
			continue
		}
		if current == nil || !strings.HasPrefix(line, "| `") {
			continue
		}
		name, _, ok := strings.Cut(strings.TrimPrefix(line, "| `"), "`")
		if ok {
			current[name] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

type keyRecorder struct {
	mu    sync.Mutex
	keys  map[string]bool
	attrs []slog.Attr
}

func (r *keyRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *keyRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.attrs {
		r.keys[a.Key] = true
	}
	rec.Attrs(func(a slog.Attr) bool {
		r.keys[a.Key] = true
		return true
	})
	return nil
}

func (r *keyRecorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &keyRecorder{keys: r.keys, attrs: append(slices.Clone(r.attrs), attrs...)}
}

func (r *keyRecorder) WithGroup(string) slog.Handler { return r }

func (r *keyRecorder) sorted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Sorted(maps.Keys(r.keys))
}
