package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
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

	var set jwk.Set
	if jwksUrlCert != "" {
		slog.Debug("jwt auth: updating trust CA")
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system cert pool: %w", err)
		}
		if ok := rootCAs.AppendCertsFromPEM([]byte(jwksUrlCert)); !ok {
			return nil, fmt.Errorf("unable to add cert to trust CA")
		}

		config := &tls.Config{
			RootCAs: rootCAs,
		}
		tr := &http.Transport{TLSClientConfig: config}
		client := &http.Client{Transport: tr}

		set, err = jwk.Fetch(ctx, jwksUrl, jwk.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch JWKS with custom CA: %w", err)
		}
	} else {
		var err error
		set, err = jwk.Fetch(ctx, jwksUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
		}
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
			slog.Debug("jwt auth: validating bearer token")
			if token, err := a.validateAuthToken(authToken); err == nil {
				ctx := NewContextWithAuth(r.Context(), token)
				r = r.WithContext(ctx)
			} else {
				slog.Warn("jwt auth: token validation failed", "error", err)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(fmt.Sprintf("ERR: %v\n", err)))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (a *JwtAuther) validateAuthToken(authToken string) (jwt.Token, error) {
	if authToken == "" {
		return nil, errors.New("empty auth token")
	}

	tokenVer, err := jwt.Parse(
		[]byte(authToken),
		jwt.WithValidate(true),
		jwt.WithKeySet(a.keySet),
		jwt.WithIssuer(a.issuer),
		jwt.WithAudience(a.clientId),
	)
	if err != nil {
		return nil, err
	}

	slog.Debug("jwt auth: token validated",
		"issuer", tokenVer.Issuer(),
		"subject", tokenVer.Subject(),
	)

	return tokenVer, nil
}
