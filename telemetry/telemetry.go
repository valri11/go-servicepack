package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

const scopeName = "github.com/valri11/go-servicepack/telemetry"

// SDK default is localhost:4317 over TLS.
const defaultEndpoint = "http://localhost:4317"

var signalEndpointEnv = []string{
	"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
}

// InitProviders initializes OTEL trace, metric, and log providers with OTLP gRPC export.
// It also configures slog with a fanout handler (stdout JSON + OTEL collector).
// When disableTelemetry is true, only the stdout slog handler is configured.
// Returns a shutdown function that flushes and closes all providers.
//
// otelEndpoint is a URL (scheme selects TLS) or bare host:port (plaintext).
// When empty, exporters read OTEL_EXPORTER_OTLP_* env.
func InitProviders(ctx context.Context,
	disableTelemetry bool,
	serviceName string,
	otelEndpoint string,
	opts ...Option,
) (func(context.Context) error, error) {
	cfg := newConfig(opts)

	stdoutHandler := traceContextHandler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.logLevel,
	})}
	slog.SetDefault(slog.New(stdoutHandler))

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	if disableTelemetry {
		slog.Info("telemetry disabled")
		return shutdown, nil
	}

	// stdout only: OTLP export errors must not loop through the OTLP logger.
	sdkLogger := slog.New(stdoutHandler)
	otel.SetLogger(logr.FromSlogHandler(stdoutHandler))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		sdkLogger.Error("otel sdk error", "error", err)
	}))

	endpointURL := resolveEndpoint(otelEndpoint)

	slog.Debug("init OTEL providers",
		"endpoint", endpointURL,
		"service", serviceName,
	)

	// exporters not yet owned by a provider
	var pending []func(context.Context) error

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) error {
		err := inErr
		for _, fn := range pending {
			err = errors.Join(err, fn(ctx))
		}
		pending = nil
		return errors.Join(err, shutdown(ctx))
	}

	res, err := newResource(ctx, serviceName, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// setup tracing

	traceProviderOptions := []trace.TracerProviderOption{
		trace.WithResource(res),
	}

	useOTLP, useConsole := exporters("OTEL_TRACES_EXPORTER")

	if useOTLP {
		var traceOpts []otlptracegrpc.Option
		if endpointURL != "" {
			traceOpts = append(traceOpts, otlptracegrpc.WithEndpointURL(endpointURL))
		}
		traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create trace exporter: %w", err))
		}
		pending = append(pending, traceExporter.Shutdown)
		traceProviderOptions = append(traceProviderOptions,
			trace.WithBatcher(traceExporter))
	}

	if useConsole {
		traceConsoleExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create trace console exporter: %w", err))
		}
		pending = append(pending, traceConsoleExporter.Shutdown)
		traceProviderOptions = append(traceProviderOptions,
			trace.WithBatcher(traceConsoleExporter))
	}

	tracerProvider := trace.NewTracerProvider(traceProviderOptions...)
	pending = nil

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// setup metrics

	metricProviderOptions := []metric.Option{
		metric.WithResource(res),
	}

	useOTLP, useConsole = exporters("OTEL_METRICS_EXPORTER")

	if useOTLP {
		var metricOpts []otlpmetricgrpc.Option
		if endpointURL != "" {
			metricOpts = append(metricOpts, otlpmetricgrpc.WithEndpointURL(endpointURL))
		}
		metricExporter, err := otlpmetricgrpc.New(ctx, metricOpts...)
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create metric exporter: %w", err))
		}
		pending = append(pending, metricExporter.Shutdown)
		metricProviderOptions = append(metricProviderOptions,
			metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		)
	}

	if useConsole {
		metricExporterConsole, err := stdoutmetric.New()
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create metric console exporter: %w", err))
		}
		pending = append(pending, metricExporterConsole.Shutdown)
		metricProviderOptions = append(metricProviderOptions,
			metric.WithReader(metric.NewPeriodicReader(metricExporterConsole)),
		)
	}

	meterProvider := metric.NewMeterProvider(
		metricProviderOptions...,
	)
	pending = nil

	otel.SetMeterProvider(meterProvider)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)

	// setup logging

	logsProviderOptions := []otelsdklog.LoggerProviderOption{
		otelsdklog.WithResource(res),
	}

	useOTLP, useConsole = exporters("OTEL_LOGS_EXPORTER")

	if useOTLP {
		var logOpts []otlploggrpc.Option
		if endpointURL != "" {
			logOpts = append(logOpts, otlploggrpc.WithEndpointURL(endpointURL))
		}
		logExporterGrpc, err := otlploggrpc.New(ctx, logOpts...)
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create log exporter: %w", err))
		}
		pending = append(pending, logExporterGrpc.Shutdown)
		logsProviderOptions = append(logsProviderOptions,
			otelsdklog.WithProcessor(otelsdklog.NewBatchProcessor(logExporterGrpc)))
	}

	if useConsole {
		logExporterConsole, err := stdoutlog.New()
		if err != nil {
			return nil, handleErr(fmt.Errorf("failed to create log console exporter: %w", err))
		}
		pending = append(pending, logExporterConsole.Shutdown)
		logsProviderOptions = append(logsProviderOptions,
			otelsdklog.WithProcessor(otelsdklog.NewBatchProcessor(logExporterConsole)))
	}

	logProvider := otelsdklog.NewLoggerProvider(logsProviderOptions...)
	pending = nil

	global.SetLoggerProvider(logProvider)
	shutdownFuncs = append(shutdownFuncs, logProvider.Shutdown)

	// create slog handler that will send log to otel collector
	otelSlogHandler := levelHandler{
		level:   cfg.logLevel,
		Handler: otelslog.NewHandler(scopeName, otelslog.WithLoggerProvider(logProvider)),
	}

	// create new logger that wraps 2 handlers
	logger := slog.New(slogmulti.Fanout(
		stdoutHandler,
		otelSlogHandler,
	))

	// set new logger as default
	slog.SetDefault(logger)

	err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))
	if err != nil {
		return nil, handleErr(err)
	}

	return shutdown, nil
}

// resolveEndpoint returns "" to let exporters read OTEL_EXPORTER_OTLP_* env.
func resolveEndpoint(explicit string) string {
	if explicit != "" {
		return withScheme(explicit)
	}
	if env := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); env != "" {
		if hasScheme(env) {
			return ""
		}
		// SDK cannot parse bare host:port.
		return withScheme(env)
	}
	for _, k := range signalEndpointEnv {
		if os.Getenv(k) != "" {
			return ""
		}
	}
	return defaultEndpoint
}

func hasScheme(endpoint string) bool {
	return strings.Contains(endpoint, "://")
}

func withScheme(endpoint string) string {
	if hasScheme(endpoint) {
		return endpoint
	}
	return "http://" + endpoint
}

func exporters(envVar string) (otlp, console bool) {
	val := os.Getenv(envVar)
	if val == "" {
		return true, false
	}
	for _, e := range strings.Split(val, ",") {
		switch strings.TrimSpace(e) {
		case "otlp":
			otlp = true
		case "console":
			console = true
		case "none", "":
		default:
			slog.Warn("unsupported telemetry exporter, ignoring", "env", envVar, "exporter", e)
		}
	}
	return otlp, console
}
