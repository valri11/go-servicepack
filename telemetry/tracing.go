package telemetry

import (
	"context"
)

// Deprecated: InitProvider only sets up tracing. Use InitProviders instead,
// which configures traces, metrics, logs, and slog in one call.
func InitProvider(ctx context.Context, enableTracing bool, serviceName string, otelEndpoint string) (func(context.Context) error, error) {
	return InitProviders(ctx, !enableTracing, serviceName, otelEndpoint)
}
