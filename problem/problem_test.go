package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func decode(t *testing.T, p *Problem) map[string]any {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestMarshalReservedMembers(t *testing.T) {
	p := New(http.StatusNotFound, "no such widget").
		WithType("https://example.com/not-found").
		WithInstance("/widgets/42")

	got := decode(t, p)

	want := map[string]any{
		"type":     "https://example.com/not-found",
		"title":    "Not Found",
		"status":   float64(404),
		"detail":   "no such widget",
		"instance": "/widgets/42",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("member %q = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d members, want %d: %v", len(got), len(want), got)
	}
}

func TestMarshalOmitsEmptyMembers(t *testing.T) {
	p := &Problem{Status: http.StatusTeapot}
	got := decode(t, p)

	for _, k := range []string{"type", "title", "detail", "instance"} {
		if _, present := got[k]; present {
			t.Errorf("member %q should be omitted when empty, got %v", k, got[k])
		}
	}
}

// RFC 9457 section 3.2: extension members are serialized at the top level of
// the problem object, not nested.
func TestMarshalFlattensExtensions(t *testing.T) {
	p := New(http.StatusBadRequest, "bad").
		With("trace_id", "abc123").
		With("errors", []InvalidParam{{Name: "age", Reason: "must be positive"}})

	got := decode(t, p)

	if got["trace_id"] != "abc123" {
		t.Errorf("trace_id = %v, want abc123", got["trace_id"])
	}

	errs, ok := got["errors"].([]any)
	if !ok {
		t.Fatalf("errors = %T, want top-level array", got["errors"])
	}
	if len(errs) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(errs))
	}
	first := errs[0].(map[string]any)
	if first["name"] != "age" || first["reason"] != "must be positive" {
		t.Errorf("errors[0] = %v", first)
	}
}

func TestExtensionsCannotOverrideReservedMembers(t *testing.T) {
	p := New(http.StatusForbidden, "real detail").
		With("detail", "spoofed").
		With("status", 200)

	got := decode(t, p)

	if got["detail"] != "real detail" {
		t.Errorf("detail = %v, reserved member must win", got["detail"])
	}
	if got["status"] != float64(403) {
		t.Errorf("status = %v, reserved member must win", got["status"])
	}
}

func TestNormalizeDefaults(t *testing.T) {
	tests := []struct {
		name       string
		in         *Problem
		wantType   string
		wantTitle  string
		wantStatus int
	}{
		{
			name:       "empty problem defaults to 500",
			in:         &Problem{},
			wantType:   DefaultType,
			wantTitle:  "Internal Server Error",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "title derived from status",
			in:         &Problem{Status: http.StatusConflict},
			wantType:   DefaultType,
			wantTitle:  "Conflict",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "explicit values preserved",
			in:         &Problem{Type: "urn:x:y", Title: "Custom", Status: http.StatusGone},
			wantType:   "urn:x:y",
			wantTitle:  "Custom",
			wantStatus: http.StatusGone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.normalize()
			if tc.in.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", tc.in.Type, tc.wantType)
			}
			if tc.in.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", tc.in.Title, tc.wantTitle)
			}
			if tc.in.Status != tc.wantStatus {
				t.Errorf("Status = %d, want %d", tc.in.Status, tc.wantStatus)
			}
		})
	}
}

func TestProblemIsAnError(t *testing.T) {
	cause := errors.New("jwks key rotation failed")
	p := Unauthorized("The provided bearer token is not valid.").WithCause(cause)

	wrapped := fmt.Errorf("auth: %w", p)

	var target *Problem
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should recover *Problem through a wrap")
	}
	if target.Status != http.StatusUnauthorized {
		t.Errorf("Status = %d, want 401", target.Status)
	}
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is should reach the attached cause")
	}
}

// The cause carries internal detail and must never appear in the response body.
func TestCauseIsNotSerialized(t *testing.T) {
	p := Internal(errors.New("postgres: password authentication failed for user \"admin\""))

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "password") {
		t.Fatalf("cause leaked into body: %s", raw)
	}
	if got := decode(t, p)["detail"]; got != genericInternalDetail {
		t.Errorf("detail = %v, want the generic message", got)
	}
}

func TestUnmarshalCollectsExtensions(t *testing.T) {
	body := `{
		"type": "https://example.com/oops",
		"title": "Oops",
		"status": 418,
		"detail": "short and stout",
		"instance": "/teapot",
		"trace_id": "deadbeef",
		"balance": 30
	}`

	var p Problem
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.Type != "https://example.com/oops" || p.Status != 418 || p.Instance != "/teapot" {
		t.Errorf("reserved members not parsed: %+v", p)
	}

	ext := p.Extensions()
	if len(ext) != 2 {
		t.Fatalf("got %d extensions, want 2: %v", len(ext), ext)
	}
	if ext["trace_id"] != "deadbeef" {
		t.Errorf("trace_id = %v", ext["trace_id"])
	}
	if ext["balance"] != float64(30) {
		t.Errorf("balance = %v", ext["balance"])
	}
}

func TestRoundTrip(t *testing.T) {
	original := New(http.StatusBadRequest, "bad input").
		WithType("https://example.com/validation").
		WithInstance("/orders").
		With("trace_id", "abc")

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Problem
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.Error() != original.Error() {
		t.Errorf("Error() = %q, want %q", back.Error(), original.Error())
	}
	if id, ok := back.TraceID(); !ok || id != "abc" {
		t.Errorf("TraceID() = %q, %v", id, ok)
	}
}

func TestUnavailableSetsRetryAfter(t *testing.T) {
	p := Unavailable("Service is starting up.", 5)
	if v, ok := p.Header("Retry-After"); !ok || v != "5" {
		t.Errorf("Retry-After = %q, %v; want 5", v, ok)
	}

	if _, ok := Unavailable("down", 0).Header("Retry-After"); ok {
		t.Error("Retry-After should be omitted when retryAfterSeconds <= 0")
	}
}

// RFC 9110 section 11.6.1 requires WWW-Authenticate on a 401.
func TestUnauthorizedSetsWWWAuthenticate(t *testing.T) {
	if _, ok := Unauthorized("nope").Header("WWW-Authenticate"); !ok {
		t.Error("401 must carry a WWW-Authenticate header")
	}
}
