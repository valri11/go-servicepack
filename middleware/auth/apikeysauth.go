package auth

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type ApiKey struct {
	User   string
	ApiKey string
	Domain string
}

type AuthInfo struct {
	User     string
	ClientId string
	Domain   string
}

type ApiKeyVerifier struct {
	apiKeys map[string]AuthInfo
}

func NewApiKeyVerifier(apiKeysFile string) (*ApiKeyVerifier, error) {
	apiKeysData, err := os.ReadFile(apiKeysFile)
	if err != nil {
		return nil, err
	}

	var apiKeys []ApiKey
	err = yaml.Unmarshal(apiKeysData, &apiKeys)
	if err != nil {
		return nil, err
	}

	apiKeysMap := make(map[string]AuthInfo)
	for _, ak := range apiKeys {
		apiKeysMap[ak.ApiKey] = AuthInfo{
			User:   ak.User,
			Domain: ak.Domain,
		}
	}

	akv := ApiKeyVerifier{
		apiKeys: apiKeysMap,
	}

	return &akv, nil
}

func (a *ApiKeyVerifier) AuthVerify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := getApiKeyAuthHeader(r.Header.Get("X-Authorization"))
		if authToken != "" {
			origin := r.Header.Get("Origin")
			log.Printf("auth token: [%s], origin: %s", authToken, origin)
			authInfo, ok := a.apiKeys[authToken]
			if ok {
				checkDomain := authInfo.Domain
				if checkDomain != "" {
					remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
					if err != nil {
						ok = false
					} else {
						log.Printf("Remote host: %s", remoteHost)
						if checkDomain == "localhost" {
							ok = (remoteHost == "127.0.0.1" || remoteHost == "::1")
						} else {
							ok = (remoteHost == checkDomain)
						}
					}
					if !ok {
						log.Printf("ApiKey verifier. Domain mismatch: %s != %s", checkDomain, r.RemoteAddr)
					}
				}
				if ok {
					authData := NewUserInfo(authInfo.User)
					ctx := context.WithValue(r.Context(), ctxAuthAccessTokenKey{}, authData)
					r = r.WithContext(ctx)
				}
			}

			if !ok {
				err := fmt.Errorf("Not authorized")
				log.Printf("ERR: %v\n", err)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(fmt.Sprintf("ERR: %v\n", err)))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func getApiKeyAuthHeader(authHeader string) string {
	if authHeader == "" {
		return ""
	}

	parts := strings.Split(authHeader, "Apikey")
	if len(parts) != 2 {
		return ""
	}

	token := strings.TrimSpace(parts[1])
	if len(token) < 1 {
		return ""
	}

	return token
}
