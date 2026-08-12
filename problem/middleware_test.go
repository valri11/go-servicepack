package problem

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovererConvertsPanicToProblem(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != MediaType {
		t.Errorf("Content-Type = %q, want %q", ct, MediaType)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body)
	}
	if body["type"] != TypePanic {
		t.Errorf("type = %v, want %q", body["type"], TypePanic)
	}
	if body["instance"] != "/widgets" {
		t.Errorf("instance = %v, want /widgets", body["instance"])
	}
}

// The panic value and stack are for the log, never the client.
func TestRecovererDoesNotLeakPanicDetail(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(errors.New("secret internal state 0xDEADBEEF"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if strings.Contains(body, "DEADBEEF") {
		t.Errorf("panic value leaked: %s", body)
	}
	if strings.Contains(body, "goroutine") {
		t.Errorf("stack trace leaked: %s", body)
	}
}

// net/http uses ErrAbortHandler to signal a deliberate abort; swallowing it
// would turn an intentional connection drop into a 500.
func TestRecovererRepanicsErrAbortHandler(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		rec := recover()
		if rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want ErrAbortHandler to propagate", rec)
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Fatal("ErrAbortHandler did not propagate")
}

func TestRecovererPassesThroughNormalResponses(t *testing.T) {
	h := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusCreated || rec.Body.String() != "ok" {
		t.Errorf("got %d %q, want 201 \"ok\"", rec.Code, rec.Body)
	}
}

func TestNotFoundHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFoundHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != MediaType {
		t.Errorf("Content-Type = %q, want %q", ct, MediaType)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body)
	}
	if body["instance"] != "/nope" {
		t.Errorf("instance = %v, want /nope", body["instance"])
	}
}

func TestMethodNotAllowedHandlerSetsAllow(t *testing.T) {
	rec := httptest.NewRecorder()
	MethodNotAllowedHandler("GET, HEAD").
		ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/widgets", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
	if v := rec.Header().Get("Allow"); v != "GET, HEAD" {
		t.Errorf("Allow = %q, want \"GET, HEAD\"", v)
	}
}
