package auth

import (
	"errors"
	"net/http"

	"github.com/valri11/go-servicepack/problem"
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
				problem.Write(r.Context(), w,
					problem.Unauthorized("This resource requires authentication.").
						WithType(problem.TypeMissingAuth).
						WithInstance(r.URL.Path).
						WithCause(errors.New("enforce auth: no auth info in context")))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
