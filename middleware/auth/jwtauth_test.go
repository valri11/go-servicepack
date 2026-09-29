package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	testIssuer   = "https://issuer.example"
	testClientID = "client-1"
)

func newTestJWKS(t *testing.T) (jwk.Key, string) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := jwk.Import(raw)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv.Set(jwk.KeyIDKey, "k1")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256())

	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	set := jwk.NewSet()
	_ = set.AddKey(pub)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(srv.Close)
	return priv, srv.URL
}

func signToken(t *testing.T, key jwk.Key, issuer string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{testClientID}).
		Subject("user-1").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Minute)).
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

func TestJwtAutherValidatesAgainstJWKS(t *testing.T) {
	key, jwksURL := newTestJWKS(t)
	a, err := NewJwtAuther(testIssuer, testClientID, jwksURL, "", "")
	if err != nil {
		t.Fatalf("NewJwtAuther: %v", err)
	}

	var gotAuth bool
	h := a.AuthVerify(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, gotAuth = AuthFromContext(r.Context())
	}))

	tests := []struct {
		name     string
		issuer   string
		wantCode int
	}{
		{"valid token", testIssuer, http.StatusOK},
		{"wrong issuer", "https://other.example", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAuth = false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+signToken(t, key, tt.issuer))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.wantCode, rec.Body)
			}
			if gotAuth != (tt.wantCode == http.StatusOK) {
				t.Errorf("auth in context = %v", gotAuth)
			}
		})
	}
}
