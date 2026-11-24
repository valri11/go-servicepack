package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func TestAuthHeaderState(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"", "absent"},
		{"   ", "absent"},
		{"Bearer abc.def.ghi", "bearer"},
		{"bearer abc", "bearer"},
		{"Bearer", "bearer without a token"},
		{"Bearer   ", "bearer without a token"},
		{"Basic dXNlcjpwYXNz", `scheme "Basic"`},
		{strings.Repeat("x", 40), `scheme "` + strings.Repeat("x", maxSchemeLen) + `..."`},
	}
	for _, tt := range tests {
		if got := authHeaderState(tt.header); got != tt.want {
			t.Errorf("authHeaderState(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestEnforceAuthReportsHeaderState(t *testing.T) {
	a := NewEnforceAuther(true)
	h := a.EnforceAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler reached without auth")
	}))

	tests := []struct {
		name       string
		header     string
		wantDetail string
	}{
		{"no header", "", "no Authorization header"},
		{"other scheme", "Basic dXNlcjpwYXNz", "used another scheme"},
		{"bearer, no token", "Bearer", "carried no token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/tile", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			if !strings.Contains(rec.Body.String(), tt.wantDetail) {
				t.Errorf("body %s does not contain %q", rec.Body, tt.wantDetail)
			}
		})
	}
}

func TestJwtAutherReportsFailureReason(t *testing.T) {
	key, jwksURL := newTestJWKS(t)
	other, _ := newTestJWKS(t)

	tests := []struct {
		name       string
		opts       []JwtAutherOption
		token      func() string
		wantReason string
	}{
		{"expired", nil, func() string {
			return signClaims(t, key, testIssuer, testClientID, time.Now().Add(-time.Hour))
		}, "the token has expired"},
		{"wrong audience", nil, func() string {
			return signClaims(t, key, testIssuer, "another-service", time.Now().Add(time.Minute))
		}, "the token was not issued for this service"},
		{"wrong issuer", nil, func() string {
			return signClaims(t, key, "https://other.example", testClientID, time.Now().Add(time.Minute))
		}, "the token was issued by an unexpected issuer"},
		{"unknown key", nil, func() string {
			return signClaims(t, other, testIssuer, testClientID, time.Now().Add(time.Minute))
		}, "the token signature could not be verified"},
		{"not a JWT", nil, func() string { return "undefined" }, "the token is not a well-formed JWT"},
		{"refresh token", []JwtAutherOption{WithRejectRefreshTokens()}, func() string {
			return signTyped(t, key, "", "refresh")
		}, "a refresh token was sent instead of an access token"},
		{"not at+jwt", []JwtAutherOption{WithRequireAccessTokenType()}, func() string {
			return signTyped(t, key, "JWT", "")
		}, "the token is not an access token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewJwtAuther(testIssuer, testClientID, jwksURL, "", "", tt.opts...)
			if err != nil {
				t.Fatalf("NewJwtAuther: %v", err)
			}
			h := a.AuthVerify(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("handler reached with an invalid token")
			}))
			req := httptest.NewRequest(http.MethodGet, "/tile", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			wantChallenge := `Bearer error="invalid_token", error_description="` + tt.wantReason + `"`
			if got := rec.Header().Get("WWW-Authenticate"); got != wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, wantChallenge)
			}
			if !strings.Contains(rec.Body.String(), tt.wantReason) {
				t.Errorf("body %s does not contain %q", rec.Body, tt.wantReason)
			}
		})
	}
}

func signClaims(t *testing.T, key jwk.Key, issuer, audience string, exp time.Time) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{audience}).
		Subject("user-1").
		IssuedAt(exp.Add(-2 * time.Minute)).
		Expiration(exp).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}
