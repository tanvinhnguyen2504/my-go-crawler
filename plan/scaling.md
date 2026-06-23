# Scaling Plan

## Current Architecture

Single-process pipeline using Go channels:

```
pageChan (chan int)
    │
[Crawler Workers x3]  ← goroutines inside one process
    │
urlChan (chan string)
    │
[Parser Workers x5]   ← goroutines inside one process
    │
bookChan (chan *book.Book)
    │
[Export — books.json]
```

All stages share memory and live-or-die together. If the process crashes, all in-flight work is lost.

---

## Target Architecture (Distributed)

Replace in-process channels with a distributed message queue so each stage is an independently deployable, horizontally scalable service.

```
Scheduler
    │
    ▼
page-queue (Redis Streams / Kafka)
    │
[Crawler Service x N]    ← scale by number of pages / sites
    │
    ▼
url-queue (Redis Streams / Kafka)
    │
[Parser Service x M]     ← bottleneck; scale this the most
    │
    ▼
book-queue (Redis Streams / Kafka)
    │
[Sink Service]           ← writes to PostgreSQL / S3
```

---

## Key Concepts to Introduce

| Concept | Replaces | Why |
|---|---|---|
| Distributed queue (Redis Streams) | `chan string` / `chan *book.Book` | Decouples services, survives restarts, allows replay |
| External store (PostgreSQL / S3) | `books.json` | Concurrent writes, queryable, durable |
| Distributed rate limiter (Redis `INCR` + TTL) | `rate.Limiter` (in-process only) | Shared across all parser replicas |
| Work deduplication (Redis Set / Bloom filter) | Nothing today | Avoid re-crawling the same URL |
| Containerization (Docker) | Single binary | Deploy N replicas independently |

---

## Interface Boundary (Code Direction)

Extract each stage behind a `Service` interface. The transport (channel vs queue) becomes a detail hidden behind the interface.

```go
// Stage 1
type CrawlerService interface {
    CrawlPage(ctx context.Context, baseURL string, pageIndex int) error
}

// Stage 2
type ParserService interface {
    ParseBook(ctx context.Context, url string) (*book.Book, error)
}

// Stage 3
type SinkService interface {
    Write(ctx context.Context, b *book.Book) error
}
```

With this boundary in place, swapping from:

```go
// local transport
urlChan <- url
```

to:

```go
// distributed transport
redisClient.XAdd(ctx, &redis.XAddArgs{Stream: "url-queue", Values: map[string]any{"url": url}})
```

...requires no change to the business logic inside each service.

---

## Migration Path

1. **Extract interfaces** — wrap each stage in a `Service` struct with the interfaces above. Pipeline logic in `main.go` barely changes.
2. **Add Redis transport** — implement a `RedisQueue` that satisfies the same interface as Go channels. Run locally with Docker.
3. **Containerize** — one `Dockerfile` per service (`crawler`, `parser`, `sink`). Use `docker-compose` to wire them together with a shared Redis instance.
4. **Distributed rate limiter** — replace `rate.Limiter` in parser with a Redis-backed limiter (e.g. `go-redis/redis_rate`) so all parser replicas share the same token bucket.
5. **Persistence** — replace `export.StreamJSON` with a `SinkService` that upserts into PostgreSQL using the book's `UPC` as the unique key (idempotent writes handle at-least-once delivery).
6. **Scale parser** — increase parser replicas (`docker-compose scale parser=10`). The rate limiter in Redis ensures the target site is not hammered regardless of replica count.

---

## Tradeoffs

**Complexity vs capacity:** A distributed queue adds operational overhead (you now operate Redis or Kafka). For a site the size of `books.toscrape.com` (50 pages, ~1000 books), the current single-process design is already sufficient.

**At-least-once delivery:** Message queues deliver messages at least once, not exactly once. The parser and sink must be idempotent — parsing the same URL twice or writing the same book twice must produce the same result. Using `UPC` as a primary key in PostgreSQL handles the sink side automatically.

**When to scale:** This architecture pays off when you target thousands of pages, multiple sites concurrently, or need fault tolerance (a crashed parser pod does not lose work — the message stays in the queue until acknowledged).
