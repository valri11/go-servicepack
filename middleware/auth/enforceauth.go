package auth

import (
	"log"
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
			if authInfo := r.Context().Value(ctxAuthAccessTokenKey{}); authInfo == nil {
				log.Printf("No auth info")
				w.WriteHeader(http.StatusUnauthorized)
				return
			} else {
				log.Printf("Auth info: %v", authInfo)
			}
		}

		next.ServeHTTP(w, r)
	})
}
