# go-servicepack

Shared building blocks for Go HTTP services: OpenTelemetry setup (`telemetry`),
RFC 9457 error responses (`problem`), metrics, auth and CORS middleware.

## Telemetry

[docs/metrics.md](docs/metrics.md) lists every metric, attribute and log key the
library emits. Custom names are constants in `semconv/`; the rest follow the
OpenTelemetry semantic conventions. `metrics/catalog_test.go` runs the
instrumentation and fails when the doc and what is emitted drift apart, so a new
metric or attribute needs a row in the doc to pass `go test`.
