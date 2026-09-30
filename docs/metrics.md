# Telemetry

What a service gets from go-servicepack when it uses `telemetry.InitProviders`,
`telemetry.HTTPMiddleware`, `telemetry.WithRequestLog`, `metrics.WithMetrics` and
`problem.Recoverer` / `problem.Write`. Names follow the
[OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/)
where one exists; the few custom names are constants in [semconv/](../semconv/semconv.go).

[metrics/catalog_test.go](../metrics/catalog_test.go) runs the instrumentation and fails
when it emits a metric, attribute or log key missing here, or when a metric listed here
is no longer emitted. Keep the first column of each table as the backquoted name.

Prometheus names: dots become underscores, counters get `_total`, units become suffixes
(`http.server.request.duration` → `http_server_request_duration_seconds_*`,
`servicepack.http.server.problems` → `servicepack_http_server_problems_total`).
`service.name` is the `job` and `service_name` label.

## Metrics

| Name | Instrument | Unit | Attributes | Source | Description |
| --- | --- | --- | --- | --- | --- |
| `servicepack.http.server.problems` | counter | `{problem}` | `http.request.method`, `http.route`, `http.response.status_code`, `problem.type` | [metrics.WithMetrics](../metrics/metrics.go) | Error responses (status ≥ 400) by cause. `problem.type` says *why*; `unclassified` means the handler wrote the error without `problem.Write`, and a rising share of it means error handling is bypassing the problem package. |
| `http.server.request.duration` | histogram | `s` | `http.request.method`, `http.route`, `http.response.status_code`, `network.protocol.*`, `server.address`, `server.port`, `url.scheme`, `problem.type` (errors only) | otelhttp via [telemetry.HTTPMiddleware](../telemetry/middleware.go) | Request latency; its `_count` is the request rate. Use it for RED: rate, error share (`http_response_status_code=~"5.."`), percentiles. |
| `http.server.request.body.size` | histogram | `By` | as `http.server.request.duration` | otelhttp | Request body size. |
| `http.server.response.body.size` | histogram | `By` | as `http.server.request.duration` | otelhttp | Response body size. |
| `go.memory.used` | updowncounter | `By` | `go.memory.type` (`stack`, `other`) | runtime instrumentation, started by `InitProviders` | Memory in use by the Go runtime. |
| `go.memory.gc.goal` | updowncounter | `By` | -- | runtime | Heap size the GC aims for; compare with `go.memory.used` to spot GC pressure. |
| `go.memory.allocated` | counter | `By` | -- | runtime | Bytes allocated on the heap; its rate is allocation throughput. |
| `go.memory.allocations` | counter | `{allocation}` | -- | runtime | Heap allocation count. |
| `go.goroutine.count` | updowncounter | `{goroutine}` | -- | runtime | Live goroutines; steady growth under constant load points at a leak. |
| `go.processor.limit` | updowncounter | `{thread}` | -- | runtime | `GOMAXPROCS`. |
| `go.config.gogc` | updowncounter | `%` | -- | runtime | `GOGC` setting. |

The runtime instrumentation also emits `go.memory.limit` when `GOMEMLIMIT` is set.

## Attributes

On metrics and on the SERVER span otelhttp starts for each routed request.

| Key | Type | Values | Set by | Notes |
| --- | --- | --- | --- | --- |
| `problem.type` | string | problem type URI, `about:blank`, `unclassified` | `problem.Write`, `metrics.WithMetrics` | On the span and on error metrics. Bounded: problem types are constants. |
| `problem.status` | int | HTTP status | `problem.Write` | Span only. 4xx leave the span status Unset, 5xx set it to Error. |
| `http.request.method` | string | RFC 9110 methods, `_OTHER` | otelhttp, `WithMetrics` | Unknown methods are normalized to `_OTHER`, so clients cannot mint series. |
| `http.route` | string | ServeMux pattern path, `unmatched` | otelhttp, `WithMetrics` | Never the raw path. Requires `HTTPMiddleware` per route inside the mux. |
| `http.response.status_code` | int | | otelhttp, `WithMetrics` | |
| `http.response.body.size` | int | | otelhttp | Span only. |
| `url.path` | string | | otelhttp | Span only (raw path is unbounded, so never on metrics). |
| `url.scheme` | string | `http`, `https` | otelhttp | |
| `server.address` | string | | otelhttp | From the `Host` header. |
| `server.port` | int | | otelhttp | When the `Host` header carries a port. |
| `client.address` | string | | otelhttp | Span only. |
| `network.peer.address` | string | | otelhttp | Span only. |
| `network.peer.port` | int | | otelhttp | Span only. |
| `network.protocol.name` | string | `http` | otelhttp | |
| `network.protocol.version` | string | `1.1`, `2` | otelhttp | |
| `user_agent.original` | string | | otelhttp | Span only, when the request has a `User-Agent`. |
| `exception.type` | string | | `problem.Write` (`span.RecordError`) | On the `exception` span event of a problem with a cause. |
| `exception.message` | string | | `problem.Write` | As above. The cause is never sent to the client. |
| `go.memory.type` | string | `stack`, `other` | runtime | `go.memory.used` only. |

## Log keys

Structured `slog` keys. Stdout JSON logs also carry `trace_id` and `span_id`; OTLP log
records carry the span context natively.

| Key | Logged by | Notes |
| --- | --- | --- |
| `http.request.method` | `WithRequestLog`, `Recoverer` | |
| `url.path` | `WithRequestLog`, `Recoverer` | |
| `http.route` | `WithRequestLog` | |
| `http.response.status_code` | `WithRequestLog` | |
| `network.protocol.version` | `WithRequestLog` | |
| `client.address` | `WithRequestLog` | |
| `duration_ms` | `WithRequestLog` | Request latency in milliseconds. `WithRequestLog` logs at debug level only. |
| `problem.type` | `problem.Write` | Warn for 4xx, error for 5xx. |
| `problem.status` | `problem.Write` | |
| `problem_detail` | `problem.Write` | Client-visible detail. |
| `problem_instance` | `problem.Write` | |
| `error` | `problem.Write`, `Recoverer` | The underlying cause; logged, never sent to the client. |
| `exception.stacktrace` | `Recoverer` | Stack of a recovered panic. |
