package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxBodyBytes caps the upstream error body read. Problem details are small;
// an unbounded read would let a misbehaving upstream exhaust memory.
const maxBodyBytes = 1 << 20

var ErrNotProblem = errors.New("problem: response is not application/problem+json")

// IsProblemResponse accepts the media type with or without parameters, for
// example "application/problem+json; charset=utf-8".
func IsProblemResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, MediaType)
}

// FromResponse parses a problem body from resp, reading and closing resp.Body.
// When the response is not problem+json it returns an error wrapping
// ErrNotProblem along with a Problem synthesized from the status line.
func FromResponse(resp *http.Response) (*Problem, error) {
	if resp == nil {
		return nil, errors.New("problem: nil response")
	}
	defer resp.Body.Close()

	if !IsProblemResponse(resp) {
		return New(resp.StatusCode, ""), fmt.Errorf("%w (got %q)",
			ErrNotProblem, resp.Header.Get("Content-Type"))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return New(resp.StatusCode, ""), fmt.Errorf("problem: read body: %w", err)
	}

	var p Problem
	if err := json.Unmarshal(body, &p); err != nil {
		return New(resp.StatusCode, ""), fmt.Errorf("problem: decode body: %w", err)
	}

	if p.Status == 0 {
		p.Status = resp.StatusCode
	}
	if p.Type == "" {
		p.Type = DefaultType
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}

	// RFC 9457 section 3.1: type is a URI reference, so it may be relative and
	// resolves against the request URL.
	if resp.Request != nil && resp.Request.URL != nil && p.Type != DefaultType {
		if ref, err := resp.Request.URL.Parse(p.Type); err == nil {
			p.Type = ref.String()
		}
	}

	return &p, nil
}

func (p *Problem) TraceID() (string, bool) {
	v, ok := p.Extension(ExtTraceID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// InvalidParams decodes the "errors" extension member. It reports false if the
// member is absent or not shaped like a validation error list.
func (p *Problem) InvalidParams() ([]InvalidParam, bool) {
	v, ok := p.Extension(ExtErrors)
	if !ok {
		return nil, false
	}

	// Typed when the problem was built locally rather than parsed.
	if params, ok := v.([]InvalidParam); ok {
		return params, true
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var params []InvalidParam
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, false
	}
	return params, true
}
