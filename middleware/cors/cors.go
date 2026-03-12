package cors

import "net/http"

// CORSConfig configures allowed origins and methods.
type CORSConfig struct {
	AllowedOrigins []string // Empty = allow all origins
	AllowedMethods string   // Default: "GET,POST,PUT,DELETE,OPTIONS"
	AllowedHeaders string   // Default: "Content-Type, X-CSRF-Token, Authorization, X-Authorization"
}

var defaultConfig = CORSConfig{
	AllowedMethods: "GET,POST,PUT,DELETE,OPTIONS",
	AllowedHeaders: "Content-Type, X-CSRF-Token, Authorization, X-Authorization",
}

// CORS is a permissive CORS middleware (allows all origins). Use NewCORS for restrictive config.
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

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := len(allowedSet) == 0 // empty = allow all
			if !allowed {
				allowed = allowedSet[origin]
			}

			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", cfg.AllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", cfg.AllowedHeaders)
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
