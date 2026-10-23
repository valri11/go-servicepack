package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/valri11/go-servicepack/problem"
)

const (
	jwksFetchTimeout        = 10 * time.Second
	jwksInitialFetchTimeout = 30 * time.Second
	jwksMinRefreshInterval  = 5 * time.Minute
)

type JwtAuther struct {
	issuer   string
	clientId string
	jwksUrl  string
	keySet   jwk.Set

	rejectRefreshTokens    bool
	requireAccessTokenType bool
}

// JwtAutherOption adjusts the checks NewJwtAuther applies on top of
// signature, iss, aud, exp and nbf.
type JwtAutherOption func(*JwtAuther)

// WithRejectRefreshTokens refuses a token carrying the claim
// "token_type": "refresh".
//
// identityserver signs refresh tokens with the same key as access tokens.
// Refresh tokens issued before October 2026 also carry the resource server's
// audience, so without this check one passes iss, aud and exp and is accepted
// as an access token for up to 30 days.
func WithRejectRefreshTokens() JwtAutherOption {
	return func(a *JwtAuther) { a.rejectRefreshTokens = true }
}

// WithRequireAccessTokenType refuses a token whose JOSE "typ" header is not
// "at+jwt" (RFC 9068 §4), which rejects ID tokens and refresh tokens as well
// as access tokens from issuers that do not set it. Turn it on only once
// every issuer the service trusts signs access tokens with at+jwt, and after
// the tokens issued before that have expired.
func WithRequireAccessTokenType() JwtAutherOption {
	return func(a *JwtAuther) { a.requireAccessTokenType = true }
}

func NewJwtAuther(issuer string,
	clientId string,
	jwksUrl string,
	jwksUrlCert string,
	signVerifyKeyPem string,
	opts ...JwtAutherOption) (*JwtAuther, error) {

	ctx := context.Background()

	client := &http.Client{Timeout: jwksFetchTimeout}
	if jwksUrlCert != "" {
		slog.Debug("jwt auth: updating trust CA")
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system cert pool: %w", err)
		}
		if ok := rootCAs.AppendCertsFromPEM([]byte(jwksUrlCert)); !ok {
			return nil, fmt.Errorf("unable to add cert to trust CA")
		}

		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{
			RootCAs: rootCAs,
		}
		client.Transport = tr
	}

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS cache: %w", err)
	}

	registerCtx, cancel := context.WithTimeout(ctx, jwksInitialFetchTimeout)
	defer cancel()
	err = cache.Register(registerCtx, jwksUrl,
		jwk.WithHTTPClient(client),
		jwk.WithMinInterval(jwksMinRefreshInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	set, err := cache.CachedSet(jwksUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached JWKS: %w", err)
	}

	a := JwtAuther{
		issuer:   issuer,
		clientId: clientId,
		jwksUrl:  jwksUrl,
		keySet:   set,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&a)
		}
	}

	return &a, nil
}

func (a *JwtAuther) AuthVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := getBearerAuthHeader(r.Header.Get("Authorization"))
		if authToken != "" {
			slog.DebugContext(r.Context(), "jwt auth: validating bearer token")
			if token, err := a.validateAuthToken(r.Context(), authToken); err == nil {
				ctx := NewContextWithAuth(r.Context(), token)
				r = r.WithContext(ctx)
			} else {
				// Validation errors name keys, issuers and clock skew, so they
				// stay in the log as the cause.
				problem.Write(r.Context(), w,
					problem.Unauthorized("The provided bearer token is not valid.").
						WithType(problem.TypeInvalidToken).
						WithInstance(r.URL.Path).
						WithCause(err))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (a *JwtAuther) validateAuthToken(ctx context.Context, authToken string) (jwt.Token, error) {
	if authToken == "" {
		return nil, errors.New("empty auth token")
	}

	tokenVer, err := jwt.Parse(
		[]byte(authToken),
		jwt.WithValidate(true),
		jwt.WithContext(ctx),
		jwt.WithKeySet(a.keySet),
		jwt.WithIssuer(a.issuer),
		jwt.WithAudience(a.clientId),
	)
	if err != nil {
		return nil, err
	}

	// Both checks run after jwt.Parse has verified the signature, so the
	// header and claims they read are the issuer's.
	if a.rejectRefreshTokens {
		var tokenType string
		if err := tokenVer.Get("token_type", &tokenType); err == nil && tokenType == "refresh" {
			return nil, errors.New("refresh token presented as an access token")
		}
	}
	if a.requireAccessTokenType {
		if err := checkAccessTokenType(authToken); err != nil {
			return nil, err
		}
	}

	issuer, _ := tokenVer.Issuer()
	subject, _ := tokenVer.Subject()
	slog.DebugContext(ctx, "jwt auth: token validated",
		"issuer", issuer,
		"subject", subject,
	)

	return tokenVer, nil
}

// checkAccessTokenType requires the JOSE "typ" header to be at+jwt. RFC 9068
// §4 allows the media-type form too, and media types are case-insensitive.
func checkAccessTokenType(compact string) error {
	msg, err := jws.Parse([]byte(compact))
	if err != nil {
		return fmt.Errorf("reading token header: %w", err)
	}
	sigs := msg.Signatures()
	if len(sigs) == 0 {
		return errors.New("token has no signature")
	}
	typ, _ := sigs[0].ProtectedHeaders().Type()
	switch strings.ToLower(typ) {
	case "at+jwt", "application/at+jwt":
		return nil
	}
	return fmt.Errorf("token typ %q is not at+jwt", typ)
}
