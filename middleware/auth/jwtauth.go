package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
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
}

func NewJwtAuther(issuer string,
	clientId string,
	jwksUrl string,
	jwksUrlCert string,
	signVerifyKeyPem string) (*JwtAuther, error) {

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

	issuer, _ := tokenVer.Issuer()
	subject, _ := tokenVer.Subject()
	slog.DebugContext(ctx, "jwt auth: token validated",
		"issuer", issuer,
		"subject", subject,
	)

	return tokenVer, nil
}
