package auth

import "testing"

func TestGetBearerAuthHeader(t *testing.T) {
	tests := map[string]string{
		"":                    "",
		"Bearer abc":          "abc",
		"bearer abc":          "abc",
		"BEARER  abc ":        "abc",
		"Bearer":              "",
		"Bearer ":             "",
		"XBearer abc":         "",
		"Basic abc":           "",
		"Bearer abcBearerxyz": "abcBearerxyz",
	}
	for in, want := range tests {
		if got := getBearerAuthHeader(in); got != want {
			t.Errorf("getBearerAuthHeader(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetApiKeyAuthHeader(t *testing.T) {
	tests := map[string]string{
		"Apikey k1":       "k1",
		"apikey k1":       "k1",
		"MyApikey k1":     "",
		"Apikey kApikeyz": "kApikeyz",
	}
	for in, want := range tests {
		if got := getApiKeyAuthHeader(in); got != want {
			t.Errorf("getApiKeyAuthHeader(%q) = %q, want %q", in, got, want)
		}
	}
}
