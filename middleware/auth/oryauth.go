package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type Auther struct {
	introspectUrl string
	clientID      string
}

func NewAuther(introspectUrl string, clientID string) *Auther {
	a := Auther{
		introspectUrl: introspectUrl,
		clientID:      clientID,
	}
	return &a
}

func (a *Auther) AuthVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := getBearerAuthHeader(r.Header.Get("Authorization"))
		if authToken != "" {
			log.Printf("auth token: [%s]", authToken)
			if authInfo, err := a.validateAuthToken(authToken); err == nil {
				ctx := context.WithValue(r.Context(), "AuthInfo", authInfo)
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

func (a *Auther) validateAuthToken(authToken string) (AuthInfo, error) {
	var authInfo AuthInfo

	if authToken == "" {
		return authInfo, errors.New("Empty auth token")
	}

	data := url.Values{
		"token": {authToken},
	}

	resp, err := http.PostForm(a.introspectUrl, data)
	if err != nil {
		return authInfo, err
	}

	var tokenInfo map[string]interface{}

	err = json.NewDecoder(resp.Body).Decode(&tokenInfo)
	if err != nil {
		return authInfo, err
	}
	log.Print(tokenInfo)

	isActive, ok := tokenInfo["active"]
	if !ok {
		return authInfo, errors.New("Invalid auth token")
	}
	active, ok := isActive.(bool)
	if !ok {
		return authInfo, errors.New("Invalid auth token")
	}
	if !active {
		return authInfo, errors.New("Expired auth token")
	}

	// validate token subject
	if tokenInfo["sub"] != a.clientID {
		return authInfo, errors.New("Invalid token")
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

// BearerAuthHeader validates incoming `r.Header.Get("Authorization")` header
// and returns token otherwise an empty string.
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
