package telemetry

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPMiddleware wraps a route with otelhttp. Install it per route (the mux
// sets r.Pattern only for the route handler), outermost in the route's chain.
func HTTPMiddleware(opts ...otelhttp.Option) func(http.Handler) http.Handler {
	opts = append([]otelhttp.Option{otelhttp.WithSpanNameFormatter(spanName)}, opts...)
	return otelhttp.NewMiddleware("", opts...)
}

func spanName(_ string, r *http.Request) string {
	route := Route(r)
	if route == "" {
		return r.Method
	}
	return r.Method + " " + route
}

// Route returns the path part of r.Pattern: "GET /items/{id}" -> "/items/{id}".
func Route(r *http.Request) string {
	route := r.Pattern
	if i := strings.IndexByte(route, '/'); i >= 0 {
		return route[i:]
	}
	return ""
}

// WithRequestLog logs each request at debug level. It no longer starts a
// span; use HTTPMiddleware for tracing.
func WithRequestLog() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if !slog.Default().Enabled(ctx, slog.LevelDebug) {
				next.ServeHTTP(w, r)
				return
			}

			m := httpsnoop.CaptureMetrics(next, w, r)

			slog.DebugContext(ctx,
				"request",
				"method", r.Method,
				"path", r.URL.Path,
				"route", Route(r),
				"status", m.Code,
				"proto", r.Proto,
				"remoteAddr", r.RemoteAddr,
				"latency_us", float64(m.Duration)/float64(time.Microsecond),
			)
		})
	}
}
