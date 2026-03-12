package auth

import (
	"log/slog"
	"net/http"
)

type EnforceAuther struct {
	enforceAuth bool
}

func NewEnforceAuther(enforceAuth bool) EnforceAuther {
	a := EnforceAuther{
		enforceAuth: enforceAuth,
	}
	return a
}

func (a *EnforceAuther) EnforceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.enforceAuth {
			if _, ok := AuthFromContext(r.Context()); !ok {
				slog.Warn("enforce auth: no auth info in context", "path", r.URL.Path)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
