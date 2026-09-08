// Package metrics define las series de tiempo que expone el sistema.
//
// Division de responsabilidades (la misma del proyecto de nivelacion):
// Postgres guarda HECHOS del dominio (este usuario aprobo este curso);
// Prometheus guarda AGREGADOS en el tiempo (cuantas aprobaciones por hora).
// No existe una tabla de metricas en la base relacional.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- API ---
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mooc_http_requests_total",
		Help: "Peticiones HTTP atendidas por la API.",
	}, []string{"method", "route", "status"})

	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mooc_http_request_duration_seconds",
		Help:    "Latencia de las peticiones HTTP (para el objetivo de p95).",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"method", "route"})

	RateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mooc_rate_limited_total",
		Help: "Peticiones rechazadas por limite de tasa.",
	}, []string{"route"})

	// --- Dominio ---
	Enrollments = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mooc_enrollments_total",
		Help: "Inscripciones creadas.",
	})

	QuizSubmissions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mooc_quiz_submissions_total",
		Help: "Intentos de quiz calificados.",
	}, []string{"result"}) // passed | failed

	BadgesIssued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mooc_badges_issued_total",
		Help: "Insignias emitidas.",
	})

	// --- Workers ---
	JobsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mooc_jobs_processed_total",
		Help: "Trabajos procesados por los workers.",
	}, []string{"task", "status"}) // status: success | retry | dead_letter | duplicate

	JobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mooc_job_duration_seconds",
		Help:    "Duracion del procesamiento de un trabajo.",
		Buckets: []float64{0.5, 1, 5, 15, 60, 300, 900, 3600},
	}, []string{"task"})

	TranscodeMinutes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mooc_transcoded_media_minutes_total",
		Help: "Minutos de video/audio transcodificados (insumo para el costo por minuto).",
	})

	UploadBytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mooc_uploaded_bytes_total",
		Help: "Bytes recibidos en el almacenamiento de objetos.",
	})
)
