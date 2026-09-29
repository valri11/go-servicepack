package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/valri11/go-servicepack/problem"
)

const introspectTimeout = 10 * time.Second

type Auther struct {
	introspectUrl string
	clientID      string
	httpClient    *http.Client
}

func NewAuther(introspectUrl string, clientID string) *Auther {
	a := Auther{
		introspectUrl: introspectUrl,
		clientID:      clientID,
		httpClient: &http.Client{
			Timeout:   introspectTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
	return &a
}

func (a *Auther) AuthVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := getBearerAuthHeader(r.Header.Get("Authorization"))
		if authToken != "" {
			slog.DebugContext(r.Context(), "ory auth: validating bearer token")
			if authInfo, err := a.validateAuthToken(r.Context(), authToken); err == nil {
				ctx := NewContextWithAuth(r.Context(), authInfo)
				r = r.WithContext(ctx)
			} else {
				// Introspection errors name the endpoint and token subject, so
				// they stay in the log as the cause.
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

func (a *Auther) validateAuthToken(ctx context.Context, authToken string) (AuthInfo, error) {
	var authInfo AuthInfo

	if authToken == "" {
		return authInfo, errors.New("empty auth token")
	}

	data := url.Values{
		"token": {authToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.introspectUrl, strings.NewReader(data.Encode()))
	if err != nil {
		return authInfo, fmt.Errorf("introspect request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return authInfo, fmt.Errorf("introspect request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return authInfo, fmt.Errorf("introspect request failed: status %d", resp.StatusCode)
	}

	var tokenInfo map[string]interface{}

	err = json.NewDecoder(resp.Body).Decode(&tokenInfo)
	if err != nil {
		return authInfo, fmt.Errorf("failed to decode introspect response: %w", err)
	}

	isActive, ok := tokenInfo["active"]
	if !ok {
		return authInfo, errors.New("invalid auth token: missing 'active' field")
	}
	active, ok := isActive.(bool)
	if !ok {
		return authInfo, errors.New("invalid auth token: 'active' is not boolean")
	}
	if !active {
		return authInfo, errors.New("expired auth token")
	}

	// validate token subject
	if tokenInfo["sub"] != a.clientID {
		return authInfo, errors.New("invalid token: subject mismatch")
	}

	userName, ok := tokenInfo["username"].(string)
	if ok {
		authInfo.User = userName
	}

	clientId, ok := tokenInfo["client_id"].(string)
	if ok {
		authInfo.ClientId = clientId
	}

	return authInfo, nil
}

// getBearerAuthHeader extracts the token from "Bearer <token>" header value.
func getBearerAuthHeader(authHeader string) string {
	return authCredentials(authHeader, "Bearer")
}
