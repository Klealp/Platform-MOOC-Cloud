// Package telemetry configura el trazado distribuido con OpenTelemetry.
//
// Division de responsabilidades de la observabilidad del proyecto:
//   - Prometheus (internal/metrics) responde "cuanto y que tan rapido" en
//     agregado (throughput, p95, jobs en DLQ).
//   - OpenTelemetry responde "que paso en ESTA peticion": la traza sigue una
//     peticion HTTP a traves de la API y sus consultas SQL, y cada trabajo del
//     worker aparece como su propio span. Ambos exportan por OTLP a Jaeger.
//
// El disenio desacopla el codigo del backend de trazado: los handlers no saben
// que existe Jaeger. Se instrumenta en los bordes (HTTP, SQL, cola) y aqui se
// elige a donde se exportan los spans. Cambiar Jaeger por otro colector OTLP es
// solo cambiar una variable de entorno.
package telemetry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ShutdownFunc vacia los spans que queden en el buffer antes de salir.
type ShutdownFunc func(context.Context) error

// Init arranca el proveedor global de trazas con exportador OTLP/HTTP.
//
// Si endpoint esta vacio, las trazas quedan DESACTIVADAS: se instala el
// propagador W3C (para que el codigo instrumentado siga funcionando) pero no se
// exporta nada. Asi el mismo binario corre con o sin observabilidad segun el
// entorno, sin recompilar.
func Init(ctx context.Context, serviceName, endpoint string, sampleRatio float64) (ShutdownFunc, error) {
	// El propagador se instala siempre: aunque las trazas esten apagadas, las
	// cabeceras traceparent se entienden si algun dia se encienden.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	// Aceptamos "host:puerto" o una URL completa; el exportador quiere host:puerto.
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimSuffix(endpoint, "/")

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(), // en local Jaeger no usa TLS
	)
	if err != nil {
		return nil, fmt.Errorf("exportador otlp: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("recurso otel: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		// ParentBased + ratio: respeta la decision del span padre (si viene de
		// otro servicio) y, para las trazas nuevas, muestrea segun el ratio.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
