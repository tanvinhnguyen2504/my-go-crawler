# Prometheus Monitoring Plan

## Goal

Add observability to the distributed pipeline using two metric layers:
- **Layer 1** — RabbitMQ native Prometheus plugin (no code required)
- **Layer 2** — Application metrics from each Go service (latency, error rate, throughput)

Visualized in Grafana using a pre-built dashboard.

---

## Checklist

- [ ] Step 1 — Enable RabbitMQ Prometheus plugin in `docker-compose.yml`
- [ ] Step 2 — Create `prometheus.yml`
- [ ] Step 3 — Add `client_golang` dependency
- [ ] Step 4 — Create `internal/metrics/metrics.go`
- [ ] Step 5 — Wire metrics into `cmd/crawler/main.go`
- [ ] Step 6 — Wire metrics into `cmd/parser/main.go`
- [ ] Step 7 — Wire metrics into `cmd/sink/main.go`
- [ ] Step 8 — Import Grafana dashboard

---

## Step 1 — Update `docker-compose.yml`

Enable the RabbitMQ Prometheus plugin, expose port `15692`, and add `prometheus` and `grafana` services.

```yaml
services:
  rabbitmq:
    image: rabbitmq:3.13-management-alpine
    environment:
      RABBITMQ_PLUGINS: "rabbitmq_management rabbitmq_prometheus"
    ports:
      - "5672:5672"
      - "15672:15672"   # Management UI
      - "15692:15692"   # Prometheus metrics endpoint
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
    depends_on:
      - rabbitmq

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
    depends_on:
      - prometheus

  scheduler:
    build: { context: ., dockerfile: cmd/scheduler/Dockerfile }
    environment: { RABBITMQ_URL: "amqp://guest:guest@rabbitmq:5672/" }
    depends_on: { rabbitmq: { condition: service_healthy } }
    restart: "no"

  crawler:
    build: { context: ., dockerfile: cmd/crawler/Dockerfile }
    environment: { RABBITMQ_URL: "amqp://guest:guest@rabbitmq:5672/" }
    ports: ["2112:2112"]
    depends_on: { rabbitmq: { condition: service_healthy } }

  parser:
    build: { context: ., dockerfile: cmd/parser/Dockerfile }
    environment: { RABBITMQ_URL: "amqp://guest:guest@rabbitmq:5672/" }
    ports: ["2113:2113"]
    depends_on: { rabbitmq: { condition: service_healthy } }

  sink:
    build: { context: ., dockerfile: cmd/sink/Dockerfile }
    environment: { RABBITMQ_URL: "amqp://guest:guest@rabbitmq:5672/" }
    ports: ["2114:2114"]
    depends_on: { rabbitmq: { condition: service_healthy } }
```

---

## Step 2 — Create `prometheus.yml`

Place at the project root.

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: rabbitmq
    static_configs:
      - targets: ["rabbitmq:15692"]

  - job_name: crawler
    static_configs:
      - targets: ["crawler:2112"]

  - job_name: parser
    static_configs:
      - targets: ["parser:2113"]

  - job_name: sink
    static_configs:
      - targets: ["sink:2114"]
```

---

## Step 3 — Add dependency

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

---

## Step 4 — Create `internal/metrics/metrics.go`

Shared package imported by all services.

```go
package metrics

import (
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
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(addr, nil)
}
```

---

## Step 5 — Wire into `cmd/crawler/main.go`

Add before `mq.Subscribe`:

```go
import (
    "github.com/my-go-crawler/internal/metrics"
    "github.com/prometheus/client_golang/prometheus"
)

go metrics.StartServer(":2112")
```

Update the handler:

```go
mq.Subscribe(ctx, queue.QueueSeeds, 1, func(d queue.Delivery) error {
    var env queue.Envelope
    json.Unmarshal(d.Body, &env)

    timer := prometheus.NewTimer(
        metrics.ProcessingDuration.WithLabelValues("crawler", env.Source),
    )
    defer timer.ObserveDuration()

    sp, ok := source.Registry[env.Source]
    if !ok {
        metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "error").Inc()
        return d.Nack(false)
    }

    urls, err := sp.Crawl(ctx, env.Payload)
    if err != nil {
        metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "error").Inc()
        return d.Nack(true)
    }

    for _, u := range urls {
        body, _ := json.Marshal(queue.Envelope{Source: env.Source, Payload: u})
        mq.Publish(ctx, queue.QueueURLs, body)
    }

    metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "success").Inc()
    return d.Ack()
})
```

---

## Step 6 — Wire into `cmd/parser/main.go`

Add before `mq.Subscribe`:

```go
go metrics.StartServer(":2113")
```

Update the handler:

```go
mq.Subscribe(ctx, queue.QueueURLs, 5, func(d queue.Delivery) error {
    var env queue.Envelope
    json.Unmarshal(d.Body, &env)

    timer := prometheus.NewTimer(
        metrics.ProcessingDuration.WithLabelValues("parser", env.Source),
    )
    defer timer.ObserveDuration()

    sp, ok := source.Registry[env.Source]
    if !ok {
        metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "error").Inc()
        return d.Nack(false)
    }

    record, err := sp.Parse(ctx, env.Payload)
    if err != nil {
        metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "error").Inc()
        return d.Nack(true)
    }

    payload, _ := json.Marshal(record.Data)
    body, _ := json.Marshal(queue.Envelope{Source: record.Source, Payload: string(payload)})
    mq.Publish(ctx, queue.QueueData, body)

    metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "success").Inc()
    return d.Ack()
})
```

---

## Step 7 — Wire into `cmd/sink/main.go`

Add before `mq.Subscribe`:

```go
go metrics.StartServer(":2114")
```

Update the handler:

```go
mq.Subscribe(ctx, queue.QueueData, 10, func(d queue.Delivery) error {
    var env queue.Envelope
    json.Unmarshal(d.Body, &env)

    timer := prometheus.NewTimer(
        metrics.ProcessingDuration.WithLabelValues("sink", env.Source),
    )
    defer timer.ObserveDuration()

    var data map[string]any
    json.Unmarshal([]byte(env.Payload), &data)

    switch env.Source {
    case "books":
        // TODO: upsert into database, unique key = data["upc"]
        log.Printf("[books] %s — %s", data["title"], data["price"])
    default:
        log.Printf("[unknown source: %s]", env.Source)
    }

    metrics.MessagesProcessed.WithLabelValues("sink", env.Source, "success").Inc()
    return d.Ack()
})
```

---

## Step 8 — Import Grafana dashboard

Go to [http://localhost:3000](http://localhost:3000) and log in with `admin / admin`.

**Add Prometheus data source:**
1. Go to **Connections → Data sources → Add new**
2. Select **Prometheus**
3. Set URL to `http://prometheus:9090`
4. Click **Save & test**

**Import RabbitMQ dashboard:**
1. Go to **Dashboards → Import**
2. Enter ID: `10991` (RabbitMQ-Overview by CloudAMQP)
3. Select the Prometheus data source
4. Click **Import**

**Useful PromQL queries for a custom dashboard:**

```promql
# Throughput per service (messages/sec)
rate(crawler_messages_total{status="success"}[1m])

# Error rate (%)
rate(crawler_messages_total{status="error"}[1m])
/ rate(crawler_messages_total[1m]) * 100

# Parser P99 latency
histogram_quantile(0.99,
  rate(crawler_processing_duration_seconds_bucket{service="parser"}[5m])
)

# Queue depth (from RabbitMQ)
rabbitmq_queue_messages{queue="urls"}

# Consumer utilisation
rabbitmq_queue_consumer_utilisation
```

---

## Verification

```bash
# Start the full stack
docker compose up --build

# Check metrics endpoints are responding
curl http://localhost:15692/metrics | grep rabbitmq_queue_messages
curl http://localhost:2112/metrics  | grep crawler_messages_total
curl http://localhost:2113/metrics  | grep crawler_messages_total
curl http://localhost:2114/metrics  | grep crawler_messages_total

# Check all Prometheus targets are UP
open http://localhost:9090/targets

# Open Grafana
open http://localhost:3000
```

---

## Makefile targets to add

```makefile
up-monitoring:
	docker compose up prometheus grafana -d

open-grafana:
	open http://localhost:3000

open-prometheus:
	open http://localhost:9090/targets

open-rabbitmq:
	open http://localhost:15672
```

---

## Port Map

| Service    | Port  | Purpose              |
|------------|-------|----------------------|
| RabbitMQ   | 5672  | AMQP                 |
| RabbitMQ   | 15672 | Management UI        |
| RabbitMQ   | 15692 | Prometheus metrics   |
| Crawler    | 2112  | /metrics             |
| Parser     | 2113  | /metrics             |
| Sink       | 2114  | /metrics             |
| Prometheus | 9090  | Query UI + scrape    |
| Grafana    | 3000  | Dashboard UI         |
