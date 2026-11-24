package auth

import (
	"fmt"
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
				// Say what the request did carry: a missing header points at
				// the client never getting a token, another scheme at a
				// misconfigured client or proxy.
				state := authHeaderState(r.Header.Get("Authorization"))
				problem.Write(r.Context(), w,
					problem.Unauthorized(missingAuthDetail(state)).
						WithType(problem.TypeMissingAuth).
						WithInstance(r.URL.Path).
						WithHeader("WWW-Authenticate", bearerChallenge("", "")).
						WithCause(fmt.Errorf("enforce auth: no auth info in context (authorization header: %s)", state)))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
