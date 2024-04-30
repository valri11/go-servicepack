package auth

type ctxAuthAccessTokenKey = struct{}

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
