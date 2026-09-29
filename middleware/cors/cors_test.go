package cors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func serve(h http.Handler, method, origin string, extra map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDefaultAllowsAnyOriginWithoutCredentials(t *testing.T) {
	rec := serve(CORS(okHandler), http.MethodGet, "https://evil.example", nil)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q, want unset", got)
	}
}

func TestCredentialsIgnoredWithoutOriginList(t *testing.T) {
	h := NewCORS(CORSConfig{AllowCredentials: true})(okHandler)
	rec := serve(h, http.MethodGet, "https://evil.example", nil)

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q, want unset", got)
	}
}

func TestAllowListReflectsOriginWithVary(t *testing.T) {
	h := NewCORS(CORSConfig{
		AllowedOrigins:   []string{"https://app.example"},
		AllowCredentials: true,
	})(okHandler)

	rec := serve(h, http.MethodGet, "https://app.example", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}

	rec = serve(h, http.MethodGet, "https://evil.example", nil)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin got Allow-Origin = %q", got)
	}
}

func TestPreflightAllowsTraceHeaders(t *testing.T) {
	rec := serve(CORS(okHandler), http.MethodOptions, "https://app.example", map[string]string{
		"Access-Control-Request-Method": http.MethodPost,
	})

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, h := range []string{"traceparent", "tracestate", "baggage"} {
		if !strings.Contains(allowed, h) {
			t.Errorf("Allow-Headers %q lacks %s", allowed, h)
		}
	}
}

func TestPlainOptionsReachesHandler(t *testing.T) {
	called := false
	h := CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	serve(h, http.MethodOptions, "https://app.example", nil)

	if !called {
		t.Error("OPTIONS without Access-Control-Request-Method was swallowed")
	}
}
