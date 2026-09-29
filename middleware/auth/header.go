package auth

import "strings"

// authCredentials returns the credentials from an "<scheme> <credentials>"
// header value, or "" when the scheme does not match. The scheme is matched
// case-insensitively, as RFC 9110 section 11.1 requires.
func authCredentials(header, scheme string) string {
	s, credentials, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(s, scheme) {
		return ""
	}
	return strings.TrimSpace(credentials)
}
