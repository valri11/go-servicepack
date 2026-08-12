package problem

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer turns a panic into a 500 problem+json. The panic value and stack
// are logged, never sent to the client.
//
// Place it after the middleware that starts the request span so RecordError
// has a span, and inside the metrics middleware so the 500 is still counted.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				"method", r.Method,
				"path", r.URL.Path,
				"stack", string(debug.Stack()),
			)

			Write(ctx, w, Internal(err).
				WithType(TypePanic).
				WithInstance(r.URL.Path))
		}()

		next.ServeHTTP(w, r)
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
