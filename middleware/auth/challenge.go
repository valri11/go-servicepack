package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

var (
	errRefreshToken = errors.New("refresh token presented as an access token")
	errTokenType    = errors.New("token typ is not at+jwt")
)

// maxSchemeLen bounds the scheme echoed into logs, so a garbage header cannot
// flood them.
const maxSchemeLen = 20

// authHeaderState describes an Authorization header for diagnostics: whether
// it was sent, and with which scheme. It never returns the credentials.
func authHeaderState(header string) string {
	h := strings.TrimSpace(header)
	if h == "" {
		return "absent"
	}
	scheme, credentials, _ := strings.Cut(h, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		if len(scheme) > maxSchemeLen {
			scheme = scheme[:maxSchemeLen] + "..."
		}
		return fmt.Sprintf("scheme %q", scheme)
	}
	if strings.TrimSpace(credentials) == "" {
		return "bearer without a token"
	}
	return "bearer"
}

// missingAuthDetail is the client-facing explanation of why a request reached
// an endpoint that requires authentication without any.
func missingAuthDetail(state string) string {
	switch {
	case state == "absent":
		return "This resource requires authentication; the request carried no Authorization header."
	case strings.HasPrefix(state, "scheme "):
		return "This resource requires a Bearer token; the Authorization header used another scheme."
	case state == "bearer without a token":
		return "This resource requires authentication; the Authorization header named Bearer but carried no token."
	default:
		return "This resource requires authentication."
	}
}

// tokenFailureReason maps a token validation error to a short reason that is
// safe to return to the client: it names the check that failed, never the
// keys, claims or values involved. The full error stays in the log.
func tokenFailureReason(err error) string {
	switch {
	case errors.Is(err, jwt.TokenExpiredError()):
		return "the token has expired"
	case errors.Is(err, jwt.TokenNotYetValidError()):
		return "the token is not valid yet"
	case errors.Is(err, jwt.InvalidAudienceError()):
		return "the token was not issued for this service"
	case errors.Is(err, jwt.InvalidIssuerError()):
		return "the token was issued by an unexpected issuer"
	case errors.Is(err, errRefreshToken):
		return "a refresh token was sent instead of an access token"
	case errors.Is(err, errTokenType):
		return "the token is not an access token"
	case errors.Is(err, jws.VerificationError()), errors.Is(err, jws.VerifyError()):
		return "the token signature could not be verified"
	case errors.Is(err, jwt.ParseError()), errors.Is(err, jws.ParseError()):
		return "the token is not a well-formed JWT"
	default:
		return "the token was rejected"
	}
}

// bearerChallenge builds an RFC 6750 section 3 WWW-Authenticate value. With no
// error code it is the bare challenge sent when no credentials were offered.
func bearerChallenge(errCode, description string) string {
	if errCode == "" {
		return "Bearer"
	}
	return fmt.Sprintf("Bearer error=%q, error_description=%q", errCode, description)
}
