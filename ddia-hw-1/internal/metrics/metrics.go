package metrics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const Requests = "http_requests_total"
const Duration = "http_request_duration_seconds"

type Route struct {
	ID string `json:"id"`
}

type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	routes   []Route
}

func New(deployment, datacenter, pod string) *Metrics {
	labels := prometheus.Labels{"deployment": deployment, "datacenter": datacenter, "pod": pod}
	m := &Metrics{registry: prometheus.NewRegistry()}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: Requests, Help: "HTTP requests.", ConstLabels: labels,
	}, []string{"endpoint", "status"})
	m.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: Duration, Help: "HTTP request duration in seconds.", ConstLabels: labels,
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 5},
	}, []string{"endpoint"})
	m.registry.MustRegister(m.requests, m.duration)
	return m
}

func (m *Metrics) Handle(mux *http.ServeMux, method, path string, handler http.HandlerFunc) {
	id := method + " " + path
	m.routes = append(m.routes, Route{ID: id})
	mux.HandleFunc(id, func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := &responseWriter{ResponseWriter: w}
		handler(response, r)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		m.requests.WithLabelValues(id, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(id).Observe(time.Since(start).Seconds())
	})
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Endpoints(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.routes)
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
