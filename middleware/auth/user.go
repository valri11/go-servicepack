package auth

import (
	"context"
)

type ctxAuthAccessTokenKey struct{}

type Claims map[string]any

type UserInfo struct {
	Username string
	Claims   Claims
}

func NewUserInfo(username string) UserInfo {
	u := UserInfo{
		Username: username,
		Claims:   make(map[string]any),
	}
	return u
}

// AuthInfo holds OAuth2/Ory introspection results.
type AuthInfo struct {
	User     string
	ClientId string
	Domain   string
}

// NewContextWithAuth stores auth data (UserInfo or AuthInfo) in context.
func NewContextWithAuth(ctx context.Context, authData any) context.Context {
	return context.WithValue(ctx, ctxAuthAccessTokenKey{}, authData)
}

// AuthFromContext retrieves auth data from context.
func AuthFromContext(ctx context.Context) (any, bool) {
	v := ctx.Value(ctxAuthAccessTokenKey{})
	return v, v != nil
}
