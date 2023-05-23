package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"google.golang.org/grpc"
)

// Initializes an OTLP exporter, and configures the corresponding trace and
// metric providers.
func InitProvider(ctx context.Context, enableTracing bool, serviceName string, otelEndpoint string) (func(context.Context) error, error) {

	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(otelEndpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
		otlptracegrpc.WithDialOption(grpc.WithTimeout(10*time.Second)),
		otlptracegrpc.WithReconnectionPeriod(5*time.Second),
	)
	traceExporter, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceName(serviceName),
		),
		resource.WithHost(),
		//resource.WithFromEnv(),
		//resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	if enableTracing {
		otel.SetTracerProvider(tracerProvider)
		// set global propagator to tracecontext (the default is no-op).
		otel.SetTextMapPropagator(propagation.TraceContext{})
	}

	done := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := traceExporter.Shutdown(ctx); err != nil {
			otel.Handle(err)
			return err
		}
		return nil
	}

	// Shutdown will flush any remaining spans and shut down the exporter.
	return done, nil
}
