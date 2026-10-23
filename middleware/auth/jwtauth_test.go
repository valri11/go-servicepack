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
	"github.com/lestrrat-go/jwx/v3/jws"
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

// signTyped signs a token for testIssuer/testClientID with the given JOSE
// "typ" header ("" leaves the jwx default, JWT) and token_type claim.
func signTyped(t *testing.T, key jwk.Key, typ, tokenType string) string {
	t.Helper()
	b := jwt.NewBuilder().
		Issuer(testIssuer).
		Audience([]string{testClientID}).
		Subject("user-1").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Minute))
	if tokenType != "" {
		b = b.Claim("token_type", tokenType)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	var signOpts []jwt.Option
	if typ != "" && typ != noTyp {
		hdrs := jws.NewHeaders()
		if err := hdrs.Set(jws.TypeKey, typ); err != nil {
			t.Fatal(err)
		}
		signOpts = append(signOpts, jws.WithProtectedHeaders(hdrs))
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), key, signOpts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

// noTyp asks for a token whose header has no "typ" at all. jwx sets JWT when
// signing a jwt.Token, so that case is signed as a raw JWS instead.
const noTyp = "-"

func TestJwtAutherTokenTypeOptions(t *testing.T) {
	key, jwksURL := newTestJWKS(t)

	tests := []struct {
		name      string
		opts      []JwtAutherOption
		typ       string
		tokenType string
		wantOK    bool
	}{
		{"default accepts a refresh token", nil, "", "refresh", true},
		{"default accepts at+jwt", nil, "at+jwt", "", true},
		{"reject refresh: refresh token refused", []JwtAutherOption{WithRejectRefreshTokens()}, "", "refresh", false},
		{"reject refresh: access token accepted", []JwtAutherOption{WithRejectRefreshTokens()}, "", "", true},
		{"require at+jwt: at+jwt accepted", []JwtAutherOption{WithRequireAccessTokenType()}, "at+jwt", "", true},
		{"require at+jwt: media type form accepted", []JwtAutherOption{WithRequireAccessTokenType()}, "application/AT+JWT", "", true},
		{"require at+jwt: JWT refused", []JwtAutherOption{WithRequireAccessTokenType()}, "JWT", "", false},
		{"require at+jwt: missing typ refused", []JwtAutherOption{WithRequireAccessTokenType()}, noTyp, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewJwtAuther(testIssuer, testClientID, jwksURL, "", "", tt.opts...)
			if err != nil {
				t.Fatalf("NewJwtAuther: %v", err)
			}
			raw := signTyped(t, key, tt.typ, tt.tokenType)
			if tt.typ == noTyp {
				raw = signWithoutTyp(t, key)
			}
			_, err = a.validateAuthToken(t.Context(), raw)
			if (err == nil) != tt.wantOK {
				t.Errorf("validateAuthToken error = %v, want ok=%v", err, tt.wantOK)
			}
		})
	}
}

// signWithoutTyp signs a valid token whose protected header has no "typ".
func signWithoutTyp(t *testing.T, key jwk.Key) string {
	t.Helper()
	payload := []byte(`{"iss":"` + testIssuer + `","aud":["` + testClientID + `"],"sub":"user-1","exp":` +
		jsonInt(time.Now().Add(time.Minute).Unix()) + `}`)
	signed, err := jws.Sign(payload, jws.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
