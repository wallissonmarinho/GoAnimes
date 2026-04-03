package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TracerName is the default OpenTelemetry tracer scope name.
const TracerName = "github.com/wallissonmarinho/GoAnimes"

func otlpDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true")
}

func otlpTracesConfigured() bool {
	if otlpDisabled() {
		return false
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

// Setup registers OTLP trace export when OTEL_* endpoints are set; otherwise JSON slog to stderr only.
func Setup(ctx context.Context) (shutdown func(context.Context) error, logger *slog.Logger, err error) {
	stderr := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	nopShutdown := func(context.Context) error { return nil }
	lg := slog.New(stderr)

	if !otlpTracesConfigured() {
		return nopShutdown, lg, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, nil, err
	}

	texp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(texp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown = func(shutdownCtx context.Context) error {
		return errors.Join(tp.Shutdown(shutdownCtx))
	}
	return shutdown, lg, nil
}

// StartSyncSpan starts a span for RSS sync jobs (scheduler, boot).
func StartSyncSpan(ctx context.Context, spanName string) (context.Context, trace.Span) {
	return otel.Tracer(TracerName).Start(ctx, spanName)
}
