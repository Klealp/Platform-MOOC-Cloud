package queuemetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Se habla con la API REST de Cloud Monitoring directamente, con el token de
// la identidad de la VM que entrega el servidor de metadatos. Asi no se
// agrega el SDK de Google (y sus decenas de dependencias) por una sola
// llamada, y no hay llaves en disco.
const (
	metadataURL        = "http://metadata.google.internal/computeMetadata/v1"
	monitoringEndpoint = "https://monitoring.googleapis.com"
)

// Publisher escribe un Snapshot como series de Cloud Monitoring.
type Publisher struct {
	Project  string
	HTTP     *http.Client
	Endpoint string                                    // vacio = monitoringEndpoint
	Token    func(ctx context.Context) (string, error) // nil = MetadataToken
}

// Publish escribe todas las series en una sola peticion. Cloud Monitoring
// crea los descriptores la primera vez (metricas GAUGE).
func (p *Publisher) Publish(ctx context.Context, s Snapshot, at time.Time) error {
	tokenFn := p.Token
	if tokenFn == nil {
		tokenFn = func(ctx context.Context) (string, error) { return MetadataToken(ctx, p.HTTP) }
	}
	token, err := tokenFn(ctx)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	body, err := json.Marshal(buildRequest(p.Project, s, at))
	if err != nil {
		return err
	}
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = monitoringEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/v3/projects/"+p.Project+"/timeSeries", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("cloud monitoring respondio %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}

// Formato JSON de projects.timeSeries.create. Los int64 viajan como texto,
// como exige la codificacion JSON de protobuf.
type timeSeriesRequest struct {
	TimeSeries []timeSeries `json:"timeSeries"`
}

type timeSeries struct {
	Metric   metric   `json:"metric"`
	Resource resource `json:"resource"`
	Points   []point  `json:"points"`
}

type metric struct {
	Type string `json:"type"`
}

type resource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

type point struct {
	Interval interval `json:"interval"`
	Value    value    `json:"value"`
}

type interval struct {
	EndTime string `json:"endTime"`
}

type value struct {
	Int64Value  *string  `json:"int64Value,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

// buildRequest arma una serie por metrica, sin etiquetas y sobre el recurso
// "global": el autoescalador necesita UNA sola serie para una metrica de
// grupo (filtro resource.type = "global").
func buildRequest(project string, s Snapshot, at time.Time) timeSeriesRequest {
	end := at.UTC().Format(time.RFC3339Nano)
	res := resource{Type: "global", Labels: map[string]string{"project_id": project}}
	entero := func(name string, n int) timeSeries {
		v := strconv.Itoa(n)
		return timeSeries{Metric: metric{Type: MetricPrefix + name}, Resource: res,
			Points: []point{{Interval: interval{EndTime: end}, Value: value{Int64Value: &v}}}}
	}
	segundos := s.OldestPending.Seconds()
	return timeSeriesRequest{TimeSeries: []timeSeries{
		entero("queue_backlog", s.Backlog()),
		entero("queue_pending", s.Pending),
		entero("queue_active", s.Active),
		entero("queue_retry", s.Retry),
		{Metric: metric{Type: MetricPrefix + "queue_oldest_pending_seconds"}, Resource: res,
			Points: []point{{Interval: interval{EndTime: end}, Value: value{DoubleValue: &segundos}}}},
	}}
}

// MetadataProject devuelve el proyecto de la VM. Falla fuera de GCE.
func MetadataProject(ctx context.Context, c *http.Client) (string, error) {
	b, err := metadataGet(ctx, c, "/project/project-id")
	return string(b), err
}

// MetadataToken devuelve un token OAuth de la identidad de la VM. Vive
// alrededor de una hora; pedirlo en cada publicacion (cada 30 s) es barato
// porque el servidor de metadatos es local y lo tiene en cache.
func MetadataToken(ctx context.Context, c *http.Client) (string, error) {
	b, err := metadataGet(ctx, c, "/instance/service-accounts/default/token")
	if err != nil {
		return "", err
	}
	var t struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(b, &t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("respuesta de token invalida")
	}
	return t.AccessToken, nil
}

func metadataGet(ctx context.Context, c *http.Client, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadatos %s respondio %d", path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}
