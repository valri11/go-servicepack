// Package semconv holds the names of the telemetry go-servicepack emits beyond
// the OpenTelemetry semantic conventions. docs/metrics.md documents them, and
// metrics/catalog_test.go keeps the two in sync with what the code emits.
package semconv

import "go.opentelemetry.io/otel/attribute"

const (
	// ProblemTypeKey is the RFC 9457 problem type URI of an error response,
	// or "unclassified" when the response did not go through problem.Write.
	ProblemTypeKey = attribute.Key("problem.type")
	// ProblemStatusKey is the HTTP status code carried by the problem details.
	ProblemStatusKey = attribute.Key("problem.status")
)

func ProblemType(val string) attribute.KeyValue {
	return ProblemTypeKey.String(val)
}

func ProblemStatus(val int) attribute.KeyValue {
	return ProblemStatusKey.Int(val)
}

// servicepack.http.server.problems counts HTTP server error responses.
const (
	ServicepackHTTPServerProblemsName        = "servicepack.http.server.problems"
	ServicepackHTTPServerProblemsUnit        = "{problem}"
	ServicepackHTTPServerProblemsDescription = "Number of HTTP server error responses (status 400 and above)."
)
