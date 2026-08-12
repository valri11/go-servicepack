// Package problem implements RFC 9457 "Problem Details for HTTP APIs".
//
// https://www.rfc-editor.org/rfc/rfc9457.html
package problem

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const MediaType = "application/problem+json"

// DefaultType is assumed when a problem has no type URI (RFC 9457 section 3.1).
const DefaultType = "about:blank"

// Problem is an RFC 9457 problem details object.
//
// Detail is client-visible and must not carry internal information. Use
// WithCause for the underlying error: it is logged and recorded on the span,
// never serialized.
type Problem struct {
	Type     string
	Title    string
	Status   int
	Detail   string
	Instance string

	ext     map[string]any
	headers map[string]string
	cause   error
}

var reserved = map[string]struct{}{
	"type": {}, "title": {}, "status": {}, "detail": {}, "instance": {},
}

type problemJSON struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func New(status int, detail string) *Problem {
	return &Problem{
		Type:   DefaultType,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}
}

func Newf(status int, format string, args ...any) *Problem {
	return New(status, fmt.Sprintf(format, args...))
}

func (p *Problem) WithType(typeURI string) *Problem {
	p.Type = typeURI
	return p
}

func (p *Problem) WithTitle(title string) *Problem {
	p.Title = title
	return p
}

func (p *Problem) WithDetail(detail string) *Problem {
	p.Detail = detail
	return p
}

func (p *Problem) WithInstance(instance string) *Problem {
	p.Instance = instance
	return p
}

// With adds an extension member. Reserved member names are ignored.
func (p *Problem) With(key string, val any) *Problem {
	if _, isReserved := reserved[key]; isReserved {
		return p
	}
	if p.ext == nil {
		p.ext = make(map[string]any, 4)
	}
	p.ext[key] = val
	return p
}

// WithHeader sets a response header written alongside the problem, such as
// WWW-Authenticate on a 401 or Retry-After on a 503.
func (p *Problem) WithHeader(key, val string) *Problem {
	if p.headers == nil {
		p.headers = make(map[string]string, 2)
	}
	p.headers[key] = val
	return p
}

// WithCause attaches the underlying error for logging and tracing. It is never
// serialized into the response body.
func (p *Problem) WithCause(err error) *Problem {
	p.cause = err
	return p
}

func (p *Problem) Extension(key string) (any, bool) {
	v, ok := p.ext[key]
	return v, ok
}

func (p *Problem) Extensions() map[string]any {
	if len(p.ext) == 0 {
		return nil
	}
	out := make(map[string]any, len(p.ext))
	for k, v := range p.ext {
		out[k] = v
	}
	return out
}

func (p *Problem) Header(key string) (string, bool) {
	v, ok := p.headers[key]
	return v, ok
}

func (p *Problem) Error() string {
	if p.Detail == "" {
		return fmt.Sprintf("%d %s", p.Status, p.Title)
	}
	return fmt.Sprintf("%d %s: %s", p.Status, p.Title, p.Detail)
}

func (p *Problem) Unwrap() error { return p.cause }

func (p *Problem) normalize() {
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	if p.Type == "" {
		p.Type = DefaultType
	}
	// RFC 9457 section 3.1: with type "about:blank", title is the status phrase.
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
}

// MarshalJSON flattens extension members into the top-level object, as
// required by RFC 9457 section 3.2.
func (p *Problem) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(problemJSON{
		Type:     p.Type,
		Title:    p.Title,
		Status:   p.Status,
		Detail:   p.Detail,
		Instance: p.Instance,
	})
	if err != nil {
		return nil, err
	}
	if len(p.ext) == 0 {
		return base, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range p.ext {
		if _, isReserved := reserved[k]; isReserved {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("problem: extension %q: %w", k, err)
		}
		merged[k] = raw
	}
	return json.Marshal(merged)
}

// UnmarshalJSON collects unrecognized top-level members as extensions.
func (p *Problem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var base problemJSON
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	p.Type = base.Type
	p.Title = base.Title
	p.Status = base.Status
	p.Detail = base.Detail
	p.Instance = base.Instance

	for k, v := range raw {
		if _, isReserved := reserved[k]; isReserved {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return fmt.Errorf("problem: extension %q: %w", k, err)
		}
		p.With(k, val)
	}
	return nil
}
