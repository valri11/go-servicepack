package cors

import (
	"log/slog"
	"net/http"
)

// CORSConfig configures allowed origins and methods.
type CORSConfig struct {
	AllowedOrigins []string // Empty = allow all origins, without credentials
	AllowedMethods string   // Default: "GET,POST,PUT,DELETE,OPTIONS"
	AllowedHeaders string   // Default: "Content-Type, X-CSRF-Token, Authorization, X-Authorization, traceparent, tracestate, baggage"

	// AllowCredentials is ignored when AllowedOrigins is empty.
	AllowCredentials bool
}

var defaultConfig = CORSConfig{
	AllowedMethods: "GET,POST,PUT,DELETE,OPTIONS",
	AllowedHeaders: "Content-Type, X-CSRF-Token, Authorization, X-Authorization, traceparent, tracestate, baggage",
}

// CORS allows all origins, without credentials. Use NewCORS for restrictive config.
func CORS(h http.Handler) http.Handler {
	return NewCORS(defaultConfig)(h)
}

// NewCORS creates a configurable CORS middleware.
func NewCORS(cfg CORSConfig) func(http.Handler) http.Handler {
	if cfg.AllowedMethods == "" {
		cfg.AllowedMethods = defaultConfig.AllowedMethods
	}
	if cfg.AllowedHeaders == "" {
		cfg.AllowedHeaders = defaultConfig.AllowedHeaders
	}

	allowedSet := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowedSet[o] = true
	}
	anyOrigin := len(allowedSet) == 0

	if anyOrigin && cfg.AllowCredentials {
		slog.Warn("cors: AllowCredentials ignored because AllowedOrigins is empty")
		cfg.AllowCredentials = false
	}

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				h.ServeHTTP(w, r)
				return
			}

			header := w.Header()
			allowed := true
			switch {
			case anyOrigin:
				header.Set("Access-Control-Allow-Origin", "*")
			case allowedSet[origin]:
				header.Set("Access-Control-Allow-Origin", origin)
				header.Add("Vary", "Origin")
				if cfg.AllowCredentials {
					header.Set("Access-Control-Allow-Credentials", "true")
				}
			default:
				header.Add("Vary", "Origin")
				allowed = false
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if allowed {
					header.Set("Access-Control-Allow-Methods", cfg.AllowedMethods)
					header.Set("Access-Control-Allow-Headers", cfg.AllowedHeaders)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
