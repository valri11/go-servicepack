package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/valri11/go-servicepack/problem"
	"github.com/valri11/go-servicepack/semconv"
	"github.com/valri11/go-servicepack/telemetry"
)

type testMetrics struct {
	reader *sdkmetric.ManualReader
	mp     *sdkmetric.MeterProvider
	app    *AppMetrics
}

func newTestMetrics(t *testing.T) testMetrics {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	app, err := NewAppMetrics(mp.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	return testMetrics{reader: reader, mp: mp, app: app}
}

func (m testMetrics) collect(t *testing.T) map[string]metricdata.Aggregation {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := m.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]metricdata.Aggregation{}
	for _, sm := range rm.ScopeMetrics {
		for _, md := range sm.Metrics {
			out[md.Name] = md.Data
		}
	}
	return out
}

func errCountPoints(t *testing.T, m testMetrics) []metricdata.DataPoint[int64] {
	t.Helper()
	data, ok := m.collect(t)[semconv.ServicepackHTTPServerProblemsName]
	if !ok {
		return nil
	}
	return data.(metricdata.Sum[int64]).DataPoints
}

func attr(set attribute.Set, key attribute.Key) string {
	v, _ := set.Value(key)
	return v.Emit()
}

func TestWithMetricsLabelsByRouteNotPath(t *testing.T) {
	m := newTestMetrics(t)

	mux := http.NewServeMux()
	mux.Handle("/", WithMetrics(m.app)(problem.NotFoundHandler()))

	for _, path := range []string{"/a", "/b/c", "/d?x=1"} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	points := errCountPoints(t, m)
	if len(points) != 1 {
		t.Fatalf("problems series = %d, want 1 (raw paths must not become labels)", len(points))
	}
	p := points[0]
	if p.Value != 3 {
		t.Errorf("count = %d, want 3", p.Value)
	}
	for key, want := range map[attribute.Key]string{
		"http.route":                "/",
		"http.request.method":       "GET",
		"http.response.status_code": "404",
		"problem.type":              problem.DefaultType,
	} {
		if got := attr(p.Attributes, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestWithMetricsUnroutedAndUnknownMethod(t *testing.T) {
	m := newTestMetrics(t)

	h := WithMetrics(m.app)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/pot/1", nil))

	points := errCountPoints(t, m)
	if len(points) != 1 {
		t.Fatalf("problems series = %d, want 1", len(points))
	}
	for key, want := range map[attribute.Key]string{
		"http.route":          unmatchedRoute,
		"http.request.method": "_OTHER",
		"problem.type":        "unclassified",
	} {
		if got := attr(points[0].Attributes, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestWithMetricsSkipsSuccess(t *testing.T) {
	m := newTestMetrics(t)

	h := WithMetrics(m.app)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if points := errCountPoints(t, m); len(points) != 0 {
		t.Errorf("problems recorded for a 200: %v", points)
	}
}

func TestWithMetricsLabelsOtelhttpMetrics(t *testing.T) {
	m := newTestMetrics(t)

	route := telemetry.HTTPMiddleware(otelhttp.WithMeterProvider(m.mp))(
		WithMetrics(m.app)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			problem.Write(r.Context(), w, problem.InvalidParams())
		})))
	mux := http.NewServeMux()
	mux.Handle("POST /items", route)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/items", nil))

	data, ok := m.collect(t)["http.server.request.duration"]
	if !ok {
		t.Fatal("otelhttp did not record http.server.request.duration")
	}
	points := data.(metricdata.Histogram[float64]).DataPoints
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1", len(points))
	}
	if got := attr(points[0].Attributes, "problem.type"); got != problem.TypeValidationError {
		t.Errorf("problem.type = %q, want %q", got, problem.TypeValidationError)
	}
	if got := attr(points[0].Attributes, "http.route"); got != "/items" {
		t.Errorf("http.route = %q, want /items", got)
	}
}
