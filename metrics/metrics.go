package metrics

import (
	"net/http"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	metricsApi "go.opentelemetry.io/otel/metric"

	"github.com/valri11/go-servicepack/problem"
	"github.com/valri11/go-servicepack/semconv"
	"github.com/valri11/go-servicepack/telemetry"
)

type AppMetrics struct {
	ReqErrCounter metricsApi.Int64Counter
}

func NewAppMetrics(meter metricsApi.Meter) (*AppMetrics, error) {
	errCounter, err := meter.Int64Counter(semconv.ServicepackHTTPServerProblemsName,
		metricsApi.WithDescription(semconv.ServicepackHTTPServerProblemsDescription),
		metricsApi.WithUnit(semconv.ServicepackHTTPServerProblemsUnit),
	)
	if err != nil {
		return nil, err
	}
	return &AppMetrics{ReqErrCounter: errCounter}, nil
}

const unmatchedRoute = "unmatched"

// WithMetrics must run per route, inside telemetry.HTTPMiddleware.
func WithMetrics(metrics *AppMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Lets problem.Write report the problem type back here, so errors
			// break down by cause and not just by status code.
			ctx, problemRec := problem.ContextWithRecorder(r.Context())
			r = r.WithContext(ctx)

			m := httpsnoop.CaptureMetrics(next, w, r)
			if m.Code < http.StatusBadRequest {
				return
			}

			problemType := problemRec.Type()
			if problemType == "" {
				// Did not go through problem.Write.
				problemType = "unclassified"
			}
			problemAttr := semconv.ProblemType(problemType)

			if labeler, ok := otelhttp.LabelerFromContext(ctx); ok {
				labeler.Add(problemAttr)
			}

			route := telemetry.Route(r)
			if route == "" {
				route = unmatchedRoute
			}
			metrics.ReqErrCounter.Add(ctx, 1, metricsApi.WithAttributes(
				attribute.String("http.request.method", normalizeMethod(r.Method)),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", m.Code),
				problemAttr,
			))
		})
	}
}

func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	}
	return "_OTHER"
}
