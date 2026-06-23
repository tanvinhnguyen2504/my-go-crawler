package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// MessagesProcessed counts total messages handled.
	// Labels: service (crawler/parser/sink), source (books/finviz/...), status (success/error)
	MessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crawler_messages_total",
			Help: "Total number of messages processed",
		},
		[]string{"service", "source", "status"},
	)

	// ProcessingDuration measures how long each message takes to process.
	// Labels: service, source
	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "crawler_processing_duration_seconds",
			Help:    "Time spent processing each message",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "source"},
	)
)

// StartServer starts an HTTP server exposing /metrics on the given addr.
// Call in a separate goroutine: go metrics.StartServer(":2112")
func StartServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("metrics server on %s: %v", addr, err)
	}
}
