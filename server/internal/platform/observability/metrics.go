package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Label values for requests outside the known sets. Both keep label
// cardinality bounded whatever the client sends.
const (
	unmatchedRoute = "unmatched"
	otherMethod    = "OTHER"
)

// standardMethods are the only method label values; net/http accepts any
// token as a method, so arbitrary ones are folded into otherMethod.
var standardMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodOptions: {},
	http.MethodConnect: {}, http.MethodTrace: {},
}

func methodLabel(method string) string {
	if _, ok := standardMethods[method]; ok {
		return method
	}
	return otherMethod
}

// Metrics owns a dedicated Prometheus registry and the HTTP instruments.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewMetrics creates a registry with Go runtime, process and HTTP metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests by method, route template and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route template.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
	)
	return m
}

// Register adds extra collectors, such as database pool statistics.
func (m *Metrics) Register(cs ...prometheus.Collector) error {
	for _, c := range cs {
		if err := m.registry.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// Middleware records request count and latency per route template.
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = unmatchedRoute
		}
		method := methodLabel(c.Request.Method)
		m.requests.WithLabelValues(method, route, strconv.Itoa(c.Writer.Status())).Inc()
		m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

// Handler serves the registry in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
