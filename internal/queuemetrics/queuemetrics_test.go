package queuemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

type fakeInspector struct {
	queues map[string]*asynq.QueueInfo
	err    error
}

func (f fakeInspector) Queues() ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for q := range f.queues {
		out = append(out, q)
	}
	return out, nil
}

func (f fakeInspector) GetQueueInfo(q string) (*asynq.QueueInfo, error) {
	info, ok := f.queues[q]
	if !ok {
		// asynq real responde con un error interno; Collect nunca debe
		// llegar aqui con una cola inexistente.
		return nil, errors.New("cola inexistente consultada")
	}
	return info, nil
}

func TestCollectSumaColasYTomaLaAntiguedadMayor(t *testing.T) {
	insp := fakeInspector{queues: map[string]*asynq.QueueInfo{
		"critical": {Pending: 1, Active: 2, Retry: 0, Latency: 3 * time.Second},
		"default":  {Pending: 4, Active: 1, Retry: 2, Latency: 40 * time.Second},
		"otra":     {Pending: 100}, // no pedida: no cuenta
	}}
	s, err := Collect(insp, []string{"critical", "default", "low"})
	if err != nil {
		t.Fatal(err)
	}
	want := Snapshot{Pending: 5, Active: 3, Retry: 2, OldestPending: 40 * time.Second}
	if s != want {
		t.Fatalf("snapshot = %+v, se esperaba %+v", s, want)
	}
	if s.Backlog() != 8 {
		t.Fatalf("backlog = %d, se esperaba 8 (pendientes + activos, sin reintentos)", s.Backlog())
	}
}

func TestCollectSinColasEsCero(t *testing.T) {
	// Recien creado Redis no existe ninguna cola: el backlog es 0, no un error.
	s, err := Collect(fakeInspector{queues: map[string]*asynq.QueueInfo{}}, []string{"critical", "default", "low"})
	if err != nil || s != (Snapshot{}) {
		t.Fatalf("Collect = %+v, %v; se esperaba cero sin error", s, err)
	}
}

func TestCollectPropagaErrorDeRedis(t *testing.T) {
	_, err := Collect(fakeInspector{err: errors.New("redis caido")}, []string{"default"})
	if err == nil {
		t.Fatal("se esperaba error si no se pueden listar las colas")
	}
}

func TestPublishEnviaUnaSerieGlobalPorMetrica(t *testing.T) {
	var got timeSeriesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/projects/proy/timeSeries" || r.Method != http.MethodPost {
			t.Errorf("peticion inesperada: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("cuerpo invalido: %v", err)
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	p := &Publisher{Project: "proy", HTTP: srv.Client(), Endpoint: srv.URL,
		Token: func(context.Context) (string, error) { return "tok", nil }}
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	s := Snapshot{Pending: 3, Active: 2, Retry: 1, OldestPending: 1500 * time.Millisecond}
	if err := p.Publish(context.Background(), s, at); err != nil {
		t.Fatal(err)
	}

	valores := map[string]string{}
	for _, ts := range got.TimeSeries {
		if ts.Resource.Type != "global" || ts.Resource.Labels["project_id"] != "proy" {
			t.Errorf("%s: recurso %+v, se esperaba global del proyecto", ts.Metric.Type, ts.Resource)
		}
		if len(ts.Points) != 1 || ts.Points[0].Interval.EndTime != "2026-10-24T12:00:00Z" {
			t.Errorf("%s: puntos %+v", ts.Metric.Type, ts.Points)
		}
		v := ts.Points[0].Value
		switch {
		case v.Int64Value != nil:
			valores[strings.TrimPrefix(ts.Metric.Type, MetricPrefix)] = *v.Int64Value
		case v.DoubleValue != nil && *v.DoubleValue != 1.5:
			t.Errorf("antiguedad = %v, se esperaba 1.5", *v.DoubleValue)
		}
	}
	want := map[string]string{"queue_backlog": "5", "queue_pending": "3", "queue_active": "2", "queue_retry": "1"}
	for k, v := range want {
		if valores[k] != v {
			t.Errorf("%s = %q, se esperaba %q", k, valores[k], v)
		}
	}
	if len(got.TimeSeries) != 5 {
		t.Errorf("series = %d, se esperaban 5", len(got.TimeSeries))
	}
}

func TestPublishDevuelveElErrorDeCloudMonitoring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"permiso denegado"}}`, http.StatusForbidden)
	}))
	defer srv.Close()
	p := &Publisher{Project: "proy", HTTP: srv.Client(), Endpoint: srv.URL,
		Token: func(context.Context) (string, error) { return "tok", nil }}
	err := p.Publish(context.Background(), Snapshot{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "permiso denegado") {
		t.Fatalf("error = %v; se esperaba el 403 con el mensaje del servicio", err)
	}
}
