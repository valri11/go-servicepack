# go-servicepack

Shared building blocks for Go HTTP services: OpenTelemetry setup (`telemetry`),
RFC 9457 error responses (`problem`), metrics, auth and CORS middleware.

## Telemetry schema (Weaver)

Everything this library emits is described in a telemetry registry in `model/`,
written in the [OpenTelemetry Weaver](https://github.com/open-telemetry/weaver)
format on top of the OTel semantic conventions (v1.44.0). It defines the custom
metric `servicepack.http.server.problems` and the `problem.*` attributes, and imports
the upstream HTTP and Go runtime metrics that otelhttp and the runtime instrumentation
emit.

From it Weaver generates:
- `semconv/`: Go constants for names, units and attribute keys, used by the code.
- `docs/telemetry/`: reference docs for every metric, span and attribute ([index](docs/telemetry/README.md)).

```
task semconv:check      # validate model/ against OTel naming policies
task semconv:generate   # regenerate semconv/ and docs/telemetry after editing model/
```

Do not edit generated files by hand. CI fails if they are out of date with `model/`.

Services define their own telemetry in their own registry, which depends on this one,
and reuse `templates/go` for code generation. basement is the reference setup (its
`model/`, `scripts/weaver.sh` and `task live-check`). Shared attributes are defined as
a V1 attribute group (`registry.problem`) so dependent registries can import them;
Weaver cannot import V2 attribute definitions yet.
