package auth

import "net/http"

type AuthVerifier interface {
	AuthVerify(next http.Handler) http.Handler
}

type noopAuthVerifier struct{}

func NewNoopVerifier() AuthVerifier {
	return noopAuthVerifier{}
}

func (a noopAuthVerifier) AuthVerify(next http.Handler) http.Handler {
	return next
}
