// Package telemetry contains code that emits telemetry (logging, metrics, tracing).
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type TracerOption func(d *customTracer)

func WithOTLPEndpoint(endpoint string) TracerOption {
	return func(d *customTracer) {
		d.endpoint = endpoint
	}
}

func WithSamplingRatio(samplingRatio float64) TracerOption {
	return func(d *customTracer) {
		d.samplingRatio = samplingRatio
	}
}

func WithAttributes(attrs ...attribute.KeyValue) TracerOption {
	return func(d *customTracer) {
		d.attributes = attrs
	}
}

type customTracer struct {
	endpoint   string
	attributes []attribute.KeyValue

	samplingRatio float64
}

func MustNewTracerProvider(opts ...TracerOption) *sdktrace.TracerProvider {
	tracer := &customTracer{
		endpoint:      "",
		attributes:    []attribute.KeyValue{},
		samplingRatio: 0,
	}

	for _, opt := range opts {
		opt(tracer)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(tracer.attributes...))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var exp sdktrace.SpanExporter
	exp, err = otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(tracer.endpoint),
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to establish a connection with the otlp exporter: %v", err))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(tracer.samplingRatio)),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(exp)),
	)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	otel.SetTracerProvider(tp)

	return tp
}

func TraceError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
