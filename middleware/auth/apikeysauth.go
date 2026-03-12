package auth

import (
	"fmt"
	"log/slog"
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

type ApiKeyVerifier struct {
	apiKeys map[string]AuthInfo
}

func NewApiKeyVerifier(apiKeysFile string) (*ApiKeyVerifier, error) {
	apiKeysData, err := os.ReadFile(apiKeysFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read API keys file %q: %w", apiKeysFile, err)
	}

	var apiKeys []ApiKey
	err = yaml.Unmarshal(apiKeysData, &apiKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API keys file %q: %w", apiKeysFile, err)
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
			slog.Debug("apikey auth: validating", "origin", origin)
			authInfo, ok := a.apiKeys[authToken]
			if ok {
				checkDomain := authInfo.Domain
				if checkDomain != "" {
					remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
					if err != nil {
						ok = false
					} else {
						if checkDomain == "localhost" {
							ok = (remoteHost == "127.0.0.1" || remoteHost == "::1")
						} else {
							ok = (remoteHost == checkDomain)
						}
					}
					if !ok {
						slog.Warn("apikey auth: domain mismatch", "expected", checkDomain, "remote", r.RemoteAddr)
					}
				}
				if ok {
					authData := NewUserInfo(authInfo.User)
					ctx := NewContextWithAuth(r.Context(), authData)
					r = r.WithContext(ctx)
				}
			}

			if !ok {
				slog.Warn("apikey auth: not authorized", "remote", r.RemoteAddr)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("ERR: not authorized\n"))
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
