package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

const serviceName = "task-queue"

var tp *sdktrace.TracerProvider

// SetTracerProvider explicitly sets the global sdktrace.TracerProvider.
func SetTracerProvider(provider *sdktrace.TracerProvider) {
	tp = provider
	if provider != nil {
		otel.SetTracerProvider(provider)
	}
}

func Start(ctx context.Context, name ...string) (context.Context, trace.Span) {
	opName := "operation"
	if len(name) > 0 && name[0] != "" {
		opName = name[0]
	}

	traceID, hasTraceID := FromContext(ctx)
	if !hasTraceID || traceID == "" {
		traceID = newID()
		ctx = context.WithValue(ctx, traceIDKey, traceID)
	}

	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		if tid, err := trace.TraceIDFromHex(traceID); err == nil {
			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    tid,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
		}
	}

	var tracer trace.Tracer
	if tp != nil {
		tracer = tp.Tracer(serviceName)
	} else {
		tracer = otel.Tracer(serviceName)
	}

	var span trace.Span
	ctx, span = tracer.Start(ctx, opName)

	if sc := span.SpanContext(); sc.IsValid() {
		traceID = sc.TraceID().String()
		ctx = context.WithValue(ctx, traceIDKey, traceID)
	}
	span.SetAttributes(attribute.String("trace.id", traceID))

	return ctx, span
}

func FromContext(ctx context.Context) (string, bool) {
	if v, ok := ctx.Value(traceIDKey).(string); ok && v != "" {
		return v, true
	}
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String(), true
	}
	return "", false
}

func Inject(ctx context.Context) string {
	id, _ := FromContext(ctx)
	return id
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, traceIDKey, traceID)
	if tid, err := trace.TraceIDFromHex(traceID); err == nil {
		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    tid,
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})
		ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	}
	return ctx
}

type ShutdownFunc func(context.Context) error

func Init(ctx context.Context, endpoint string) (ShutdownFunc, error) {
	if endpoint == "" {
		return func(_ context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tp = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown tracer provider: %w", err)
		}
		return nil
	}, nil
}

func ForceFlush(ctx context.Context) {
	if tp != nil {
		_ = tp.ForceFlush(ctx)
	}
}

var newID = func() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}
