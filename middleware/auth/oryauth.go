package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
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
			Timeout: introspectTimeout,
		},
	}
	return &a
}

func (a *Auther) AuthVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := getBearerAuthHeader(r.Header.Get("Authorization"))
		if authToken != "" {
			slog.Debug("ory auth: validating bearer token")
			if authInfo, err := a.validateAuthToken(authToken); err == nil {
				ctx := NewContextWithAuth(r.Context(), authInfo)
				r = r.WithContext(ctx)
			} else {
				slog.Warn("ory auth: token validation failed", "error", err)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(fmt.Sprintf("ERR: %v\n", err)))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Auther) validateAuthToken(authToken string) (AuthInfo, error) {
	var authInfo AuthInfo

	if authToken == "" {
		return authInfo, errors.New("empty auth token")
	}

	data := url.Values{
		"token": {authToken},
	}

	resp, err := a.httpClient.PostForm(a.introspectUrl, data)
	if err != nil {
		return authInfo, fmt.Errorf("introspect request failed: %w", err)
	}
	defer resp.Body.Close()

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
	if authHeader == "" {
		return ""
	}

	parts := strings.Split(authHeader, "Bearer")
	if len(parts) != 2 {
		return ""
	}

	token := strings.TrimSpace(parts[1])
	if len(token) < 1 {
		return ""
	}

	return token
}
