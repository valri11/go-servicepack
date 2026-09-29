package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

type Option func(*config)

type config struct {
	logLevel       slog.Leveler
	serviceVersion string
	environment    string
	attrs          []attribute.KeyValue
}

func newConfig(opts []Option) config {
	cfg := config{logLevel: slog.LevelInfo}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithLogLevel sets the stdout and OTLP log level. Default Info.
func WithLogLevel(level slog.Leveler) Option {
	return func(c *config) {
		if level != nil {
			c.logLevel = level
		}
	}
}

// WithServiceVersion sets service.version. Default: build info version.
func WithServiceVersion(version string) Option {
	return func(c *config) { c.serviceVersion = version }
}

func WithEnvironment(env string) Option {
	return func(c *config) { c.environment = env }
}

func WithResourceAttributes(attrs ...attribute.KeyValue) Option {
	return func(c *config) { c.attrs = append(c.attrs, attrs...) }
}

func newResource(ctx context.Context, serviceName string, cfg config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		// the service name used to display traces in backends
		semconv.ServiceName(serviceName),
	}
	version := cfg.serviceVersion
	if version == "" {
		version = buildVersion()
	}
	if version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	if cfg.environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(cfg.environment))
	}
	attrs = append(attrs, cfg.attrs...)

	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithContainer(),
		// not WithProcess: command line may carry secrets
		resource.WithProcessPID(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	if errors.Is(err, resource.ErrPartialResource) {
		slog.Warn("telemetry resource partially detected", "error", err)
		return res, nil
	}
	return res, err
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return ""
	}
	return info.Main.Version
}
