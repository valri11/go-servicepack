package problem

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/felixge/httpsnoop"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Recoverer turns a panic into a 500 problem+json. The panic value and stack
// are logged, never sent to the client.
//
// Place it inside otelhttp and the metrics middleware. If the response has
// already started, the panic is recorded and the response aborted.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var started bool
		tw := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if code >= http.StatusOK {
						started = true
					}
					next(code)
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(b []byte) (int, error) {
					started = true
					return next(b)
				}
			},
			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					started = true
					return next(src)
				}
			},
			Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() {
					started = true
					next()
				}
			},
		})

		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			// net/http uses ErrAbortHandler to signal a deliberate abort.
			if rec == http.ErrAbortHandler {
				panic(rec)
			}

			err, ok := rec.(error)
			if !ok {
				err = fmt.Errorf("panic: %v", rec)
			}

			ctx := r.Context()
			slog.ErrorContext(ctx, "panic recovered",
				"error", err,
				string(otelsemconv.HTTPRequestMethodKey), r.Method,
				string(otelsemconv.URLPathKey), r.URL.Path,
				string(otelsemconv.ExceptionStacktraceKey), string(debug.Stack()),
			)

			p := Internal(err).
				WithType(TypePanic).
				WithInstance(r.URL.Path)

			if started {
				p.normalize()
				observe(ctx, p)
				panic(http.ErrAbortHandler)
			}

			Write(ctx, w, p)
		}()

		next.ServeHTTP(tw, r)
	})
}

// NotFoundHandler responds with a 404 problem+json. Register it on "/" so
// unmatched routes return problem details instead of the text/plain body
// http.NotFound writes.
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Write(r.Context(), w, NotFound("The requested resource does not exist.").
			WithInstance(r.URL.Path))
	})
}

func MethodNotAllowedHandler(allowed string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Write(r.Context(), w, MethodNotAllowed(allowed).WithInstance(r.URL.Path))
	})
}
