package tasks

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInterruptedConContextoVivo(t *testing.T) {
	if err := interrupted(context.Background()); err != nil {
		t.Fatalf("interrupted = %v, se esperaba nil con el contexto vivo", err)
	}
}

func TestInterruptedAlRetirarElWorker(t *testing.T) {
	// asynq cancela el contexto cuando devuelve el trabajo a la cola al
	// apagarse el worker.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := interrupted(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted = %v, se esperaba un error que envuelva context.Canceled", err)
	}
}

func TestInterruptedPorTimeoutDelTrabajo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if err := interrupted(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("interrupted = %v, se esperaba context.DeadlineExceeded", err)
	}
}

func TestFailAssetNoTocaLaBaseSiFueInterrumpido(t *testing.T) {
	// Deps sin base de datos: si failAsset/failTranscode intentaran escribir,
	// fallarian con un panic por puntero nulo. Con el contexto cancelado deben
	// salir antes, sin marcar el asset como fallido.
	d := &Deps{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.failAsset(ctx, "a1", "scan:a1", "ffprobe murio"); !errors.Is(err, context.Canceled) {
		t.Fatalf("failAsset = %v, se esperaba context.Canceled", err)
	}
	if err := d.failTranscode(ctx, "a1", "hls:a1", "ffmpeg murio"); !errors.Is(err, context.Canceled) {
		t.Fatalf("failTranscode = %v, se esperaba context.Canceled", err)
	}
}
