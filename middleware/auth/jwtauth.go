package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
		log.Println("updating trust CA")
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		if ok := rootCAs.AppendCertsFromPEM([]byte(jwksUrlCert)); !ok {
			return nil, fmt.Errorf("Unable to add cert to trust CA")
		}

		config := &tls.Config{
			//InsecureSkipVerify: *insecure,
			RootCAs: rootCAs,
		}
		tr := &http.Transport{TLSClientConfig: config}
		client := &http.Client{Transport: tr}

		set, err = jwk.Fetch(ctx, jwksUrl, jwk.WithHTTPClient(client))
		if err != nil {
			return nil, err
		}
	} else {
		// Use jwk.Cache if you intend to keep reuse the JWKS over and over
		var err error
		set, err = jwk.Fetch(ctx, jwksUrl)
		if err != nil {
			return nil, err
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
			log.Printf("auth token: [%s]", authToken)
			if authInfo, err := a.validateAuthToken(authToken); err == nil {
				ctx := context.WithValue(r.Context(), AuthUserContextKey, authInfo)
				r = r.WithContext(ctx)
			} else {
				log.Printf("ERR: %v\n", err)
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
		return nil, errors.New("Empty auth token")
	}

	// Validate using public key
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

	buf, err := json.MarshalIndent(tokenVer, "", "  ")
	if err != nil {
		return nil, err
	}
	log.Printf("%s\n", buf)

	return tokenVer, nil
}
