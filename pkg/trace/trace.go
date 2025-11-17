package trace

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the name of the tracer used throughout the application
	TracerName = "github.com/realtime-ai/realtime-ai"
)

var (
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	mu             sync.RWMutex
)

// Config holds the configuration for tracing
type Config struct {
	// ServiceName is the name of the service
	ServiceName string
	// ServiceVersion is the version of the service
	ServiceVersion string
	// Environment is the deployment environment (dev, staging, prod)
	Environment string
	// ExporterType defines which exporter to use: "stdout", "otlp", or "none"
	ExporterType string
	// OTLPEndpoint is the endpoint for OTLP exporter (e.g., "localhost:4317")
	OTLPEndpoint string
	// SamplingRate is the rate at which traces are sampled (0.0 to 1.0)
	SamplingRate float64
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		ServiceName:  "realtime-ai",
		ServiceVersion: "0.1.0",
		Environment:  getEnv("ENVIRONMENT", "development"),
		ExporterType: getEnv("TRACE_EXPORTER", "stdout"),
		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		SamplingRate: 1.0, // Sample all traces by default
	}
}

// Initialize sets up the global tracer provider
func Initialize(ctx context.Context, cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	if tracerProvider != nil {
		return fmt.Errorf("tracer provider already initialized")
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration
	var exporter sdktrace.SpanExporter
	switch cfg.ExporterType {
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	case "otlp":
		client := otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(), // Use WithTLSCredentials() for production
		)
		exporter, err = otlptrace.New(ctx, client)
		if err != nil {
			return fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	case "none":
		// No-op exporter for when tracing is disabled
		exporter = &noopExporter{}
	default:
		return fmt.Errorf("unsupported exporter type: %s", cfg.ExporterType)
	}

	// Create tracer provider with sampler
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRate))

	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tracerProvider)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// Create tracer
	tracer = tracerProvider.Tracer(TracerName)

	log.Printf("Tracing initialized with exporter: %s", cfg.ExporterType)
	return nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if tracerProvider == nil {
		return nil
	}

	if err := tracerProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown tracer provider: %w", err)
	}

	tracerProvider = nil
	tracer = nil
	return nil
}

// GetTracer returns the global tracer
func GetTracer() trace.Tracer {
	mu.RLock()
	defer mu.RUnlock()

	if tracer == nil {
		// Return a no-op tracer if not initialized
		return otel.Tracer(TracerName)
	}
	return tracer
}

// StartSpan is a convenience function to start a new span
func StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, spanName, opts...)
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// noopExporter is a no-op exporter used when tracing is disabled
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}
