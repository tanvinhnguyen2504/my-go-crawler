# Retry & Error Handling — Tracking Plan

## Tổng quan

Mỗi lớp xử lý lỗi độc lập nhau:

```
[startup]      amqp.Dial → RetryWithBackoff (5 lần, 500ms→30s)
[HTTP]         c.Visit   → visitWithRetry   (3 lần, 1s→10s, 15s timeout)
[message]      Crawl/Parse fail → Envelope.Attempt++ → re-publish với backoff
[dead-letter]  Attempt >= 5 → "dead" queue → DeadLetter()
```

---

## Checklist

### Step 1 — `pkg/utils.go`
- [x] `RetryWithBackoff(ctx, maxAttempts, baseDelay, maxDelay, isTransient, fn)` 
- [x] `IsTransientNetwork(err)` — timeout, EOF, connection reset

### Step 2 — `internal/queue/queue.go`
- [x] `QueueDead = "dead"` 
- [x] `Envelope.Attempt int json:"attempt,omitempty"`

### Step 3 — `internal/queue/rabbitmq.go`
- [x] `New()` — wrap `amqp.Dial` với `RetryWithBackoff` (5 lần, 500ms→30s)
- [x] `DeadLetter(ctx, env, reason)` — publish vào `QueueDead` kèm reason
- [x] `mustJSON()` helper

### Step 4 — `internal/source/books/books.go`
- [ ] `bookLimiter` — package-level `rate.NewLimiter(rate.Every(500ms), 1)` thay `rate.Inf`
- [ ] `visitWithRetry(ctx, c, url)` — dùng `RetryWithBackoff` (3 lần, 1s→10s)
- [ ] `Crawl()` — thêm `c.SetRequestTimeout(15s)`, dùng `visitWithRetry`
- [ ] `scrapeBook()` — thêm `c.SetRequestTimeout(15s)`, dùng `visitWithRetry`

### Step 5 — `cmd/crawler/main.go`
- [ ] `DeclareQueue(QueueDead)` khi startup
- [ ] Thay `Nack(true)` bằng envelope retry:
  - Tăng `env.Attempt++`
  - Nếu `Attempt < 5`: sleep backoff → `Publish` lại → `Ack`
  - Nếu `Attempt >= 5`: `DeadLetter()` → `Ack`

### Step 6 — `cmd/parser/main.go`
- [ ] `DeclareQueue(QueueDead)` khi startup
- [ ] Thay `Nack(true)` bằng envelope retry (giống crawler, target queue là `QueueURLs`)

---

## Backoff delays

| Attempt | Crawler/Parser delay | Dial delay   | HTTP delay |
|---------|---------------------|--------------|------------|
| 0 → 1   | 2s                  | 500ms        | 1s         |
| 1 → 2   | 4s                  | 1s           | 2s         |
| 2 → 3   | 8s                  | 2s           | —(max 3)   |
| 3 → 4   | 16s                 | 4s           |            |
| 4 → 5   | 32s → dead letter   | 8s           |            |
| cap     | 60s                 | 30s          | 10s        |

---

## Dead letter format

Message trong queue `dead`:
```json
{
  "source": "books",
  "payload": "{\"original\":\"https://...\",\"reason\":\"dial tcp: i/o timeout\"}",
  "attempt": 5
}
```

---

## Progress

| File | Status |
|------|--------|
| `pkg/utils.go` | ✅ Done |
| `internal/queue/queue.go` | ✅ Done |
| `internal/queue/rabbitmq.go` | ✅ Done |
| `internal/source/books/books.go` | ⏳ Pending |
| `cmd/crawler/main.go` | ⏳ Pending |
| `cmd/parser/main.go` | ⏳ Pending |
