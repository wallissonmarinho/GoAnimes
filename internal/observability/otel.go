package observability

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Setup configures OpenTelemetry tracing using OTLP over HTTP.
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	if isDisabled() {
		return func(context.Context) error { return nil }, nil
	}
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "goanimes"
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown, nil
}

func isDisabled() bool {
	v := strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED"))
	return strings.EqualFold(v, "true") || v == "1"
}
