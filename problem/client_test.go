package problem

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// roundTrip serves handler over a test server and returns the parsed problem.
func roundTrip(t *testing.T, handler http.HandlerFunc) (*Problem, error) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/widgets/42")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return FromResponse(resp)
}

func TestFromResponseParsesProblem(t *testing.T) {
	p, err := roundTrip(t, func(w http.ResponseWriter, r *http.Request) {
		Write(r.Context(), w, NotFound("no such widget").
			WithType("https://example.com/not-found").
			WithInstance("/widgets/42").
			With("trace_id", "cafe"))
	})
	if err != nil {
		t.Fatalf("FromResponse: %v", err)
	}

	if p.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", p.Status)
	}
	if p.Type != "https://example.com/not-found" {
		t.Errorf("Type = %q", p.Type)
	}
	if p.Detail != "no such widget" {
		t.Errorf("Detail = %q", p.Detail)
	}
	if id, ok := p.TraceID(); !ok || id != "cafe" {
		t.Errorf("TraceID() = %q, %v; want cafe", id, ok)
	}
}

func TestFromResponseRejectsNonProblemContentType(t *testing.T) {
	p, err := roundTrip(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "plain old error", http.StatusBadGateway)
	})

	if !errors.Is(err, ErrNotProblem) {
		t.Fatalf("err = %v, want ErrNotProblem", err)
	}
	// A usable problem is still synthesized from the status line.
	if p == nil || p.Status != http.StatusBadGateway {
		t.Errorf("fallback problem = %+v, want status 502", p)
	}
}

func TestFromResponseAcceptsContentTypeParameters(t *testing.T) {
	_, err := roundTrip(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", MediaType+"; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"title":"Conflict","status":409}`))
	})
	if err != nil {
		t.Fatalf("FromResponse should accept a charset parameter: %v", err)
	}
}

// RFC 9457 section 3.1: type is a URI reference and may be relative.
func TestFromResponseResolvesRelativeType(t *testing.T) {
	p, err := roundTrip(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"type":"/problems/expired","status":410}`))
	})
	if err != nil {
		t.Fatalf("FromResponse: %v", err)
	}

	if got := p.Type; got == "/problems/expired" {
		t.Error("relative type was not resolved against the request URL")
	} else if !strings.HasSuffix(got, "/problems/expired") {
		t.Errorf("Type = %q, want an absolute URL ending in /problems/expired", got)
	}
}

func TestFromResponseFillsMissingMembers(t *testing.T) {
	p, err := roundTrip(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{}`))
	})
	if err != nil {
		t.Fatalf("FromResponse: %v", err)
	}

	if p.Status != http.StatusServiceUnavailable {
		t.Errorf("Status = %d, want it filled from the status line", p.Status)
	}
	if p.Title != "Service Unavailable" {
		t.Errorf("Title = %q, want it derived from the status", p.Title)
	}
	if p.Type != DefaultType {
		t.Errorf("Type = %q, want %q", p.Type, DefaultType)
	}
}

func TestFromResponseNilResponse(t *testing.T) {
	if _, err := FromResponse(nil); err == nil {
		t.Error("FromResponse(nil) should return an error")
	}
}

func TestInvalidParamsRoundTripThroughWire(t *testing.T) {
	p, err := roundTrip(t, func(w http.ResponseWriter, r *http.Request) {
		Write(r.Context(), w, InvalidParams(
			InvalidParam{Name: "age", Reason: "must be positive"},
			InvalidParam{Name: "email", Reason: "must be a valid address"},
		))
	})
	if err != nil {
		t.Fatalf("FromResponse: %v", err)
	}

	params, ok := p.InvalidParams()
	if !ok {
		t.Fatal("InvalidParams() should decode the errors extension")
	}
	if len(params) != 2 {
		t.Fatalf("got %d params, want 2: %v", len(params), params)
	}
	if params[0].Name != "age" || params[1].Reason != "must be a valid address" {
		t.Errorf("params = %v", params)
	}
}

// A locally built problem holds typed values rather than decoded JSON.
func TestInvalidParamsOnLocallyBuiltProblem(t *testing.T) {
	p := InvalidParams(InvalidParam{Name: "sku", Reason: "unknown"})

	params, ok := p.InvalidParams()
	if !ok || len(params) != 1 || params[0].Name != "sku" {
		t.Errorf("InvalidParams() = %v, %v", params, ok)
	}
}

func TestInvalidParamsAbsent(t *testing.T) {
	if _, ok := NotFound("nope").InvalidParams(); ok {
		t.Error("InvalidParams() should report false when the extension is absent")
	}
}
