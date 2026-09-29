package telemetry

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
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
				string(semconv.HTTPRequestMethodKey), r.Method,
				string(semconv.URLPathKey), r.URL.Path,
				string(semconv.HTTPRouteKey), Route(r),
				string(semconv.HTTPResponseStatusCodeKey), m.Code,
				string(semconv.NetworkProtocolVersionKey), fmt.Sprintf("%d.%d", r.ProtoMajor, r.ProtoMinor),
				string(semconv.ClientAddressKey), clientAddress(r),
				"duration_ms", float64(m.Duration)/float64(time.Millisecond),
			)
		})
	}
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
