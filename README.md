# my-go-crawler

Go web crawler với 2 chế độ chạy:

- **Local mode** — single process, dùng Go channels, không cần hạ tầng
- **Distributed mode** — pipeline phân tán qua RabbitMQ, mỗi stage là service độc lập

## Kiến trúc distributed

```
[Scheduler] → queue:seeds → [Crawler] → queue:urls → [Parser] → queue:data → [Sink]
```

Mỗi service scale độc lập. Thêm source mới chỉ cần implement `SourceParser` trong `internal/source/` — không chạm infrastructure.

---

## Yêu cầu

- Go 1.25+
- Docker + Docker Compose (chỉ cần cho distributed mode)

---

## Cài đặt

```bash
git clone <repo-url>
cd my-go-crawler
go mod download
```

---

## Chế độ 1: Local (single process)

Crawl `books.toscrape.com`, ghi kết quả ra `books.json`.

```bash
go run ./main.go
```

Output: file `books.json` trong thư mục hiện tại.

---

## Chế độ 2: Distributed (RabbitMQ)

### Bước 1 — Cấu hình môi trường

```bash
cp .env.example .env
```

Mở `.env` và đặt giá trị (mặc định dùng được ngay nếu chạy local):

```env
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

### Bước 2 — Khởi động RabbitMQ

```bash
docker compose up rabbitmq -d
```

Kiểm tra RabbitMQ healthy:

```bash
docker compose ps
# rabbitmq phải hiện trạng thái: healthy
```

**Management UI:** [http://localhost:15672](http://localhost:15672) — đăng nhập `guest / guest`

### Bước 3 — Chạy 4 services (mở 4 terminal riêng)

Thứ tự khuyến nghị: khởi động sink và parser trước để sẵn sàng nhận message.

**Terminal 1 — Sink**
```bash
go run ./cmd/sink
```

**Terminal 2 — Parser**
```bash
go run ./cmd/parser
```

**Terminal 3 — Crawler**
```bash
go run ./cmd/crawler
```

**Terminal 4 — Scheduler** (chạy một lần, seed xong tự thoát)
```bash
go run ./cmd/scheduler
```

Sau khi scheduler chạy xong, pipeline tự hoạt động:

1. **Crawler** nhận seed URL → crawl listing page → publish book URLs vào `queue:urls`
2. **Parser** nhận book URL → parse chi tiết → publish data vào `queue:data`
3. **Sink** nhận data → log ra terminal *(TODO: lưu vào DB)*

### Bước 4 — Theo dõi queue

Vào tab **Queues** tại [http://localhost:15672](http://localhost:15672):

| Queue | Nội dung |
|---|---|
| `seeds` | Listing page URLs chờ crawler xử lý |
| `urls` | Book detail URLs chờ parser xử lý |
| `data` | Parsed records chờ sink lưu trữ |

### Chạy toàn bộ bằng Docker

```bash
docker compose up --build
```

Scale parser lên nhiều replicas (xử lý song song):

```bash
docker compose up --scale parser=5
```

---

## Thêm source mới

1. Tạo file `internal/source/<name>.go`
2. Implement interface `SourceParser`:

```go
package source

import "context"

func init() { Register("<name>", &mySource{}) }

type mySource struct{}

func (s *mySource) SeedURLs() []string {
    return []string{"https://example.com/listing"}
}

func (s *mySource) Crawl(ctx context.Context, seedURL string) ([]string, error) {
    // thu thập item URLs từ listing page
    return []string{}, nil
}

func (s *mySource) Parse(ctx context.Context, url string) (*Record, error) {
    // extract data từ item page
    return &Record{Source: "<name>", Data: map[string]any{}}, nil
}
```

3. Thêm `case "<name>":` trong `cmd/sink/main.go` để xử lý data đến

**Không cần thay đổi gì** ở queue, crawler service, hay parser service.

Source hiện có:

| Source | File | Trạng thái |
|---|---|---|
| `books` | `internal/source/books.go` | Hoàn chỉnh — crawl `books.toscrape.com` |
| `finviz` | `internal/source/finviz.go` | Stub — chờ implement |
| `coingecko` | `internal/source/coingecko.go` | Stub — chờ implement |

---

## Cấu trúc project

```
my-go-crawler/
├── main.go                          # Local mode entry point
├── cmd/
│   ├── scheduler/
│   │   ├── main.go                  # Seed queue rồi thoát
│   │   └── Dockerfile
│   ├── crawler/
│   │   ├── main.go                  # Crawl listing pages → URLs
│   │   └── Dockerfile
│   ├── parser/
│   │   ├── main.go                  # Parse item pages → data
│   │   └── Dockerfile
│   └── sink/
│       ├── main.go                  # Nhận data → lưu trữ
│       └── Dockerfile
├── internal/
│   ├── book/book.go                 # Book struct
│   ├── crawler/crawler.go           # HTML crawler (colly)
│   ├── parser/parser.go             # Book detail parser + rate limiter
│   ├── export/export.go             # JSON exporter (local mode)
│   ├── queue/
│   │   ├── queue.go                 # Producer/Consumer interfaces + queue constants
│   │   └── rabbitmq.go              # AMQP implementation
│   └── source/
│       ├── source.go                # SourceParser interface + registry
│       ├── books.go                 # books.toscrape.com strategy
│       ├── finviz.go                # Finviz stub
│       └── coingecko.go             # CoinGecko stub
├── config/
│   └── config.go                    # EnvOr helper
├── docker-compose.yml               # RabbitMQ + 4 app services
├── .env.example                     # Template biến môi trường
└── plan/
    └── scaling.md                   # Tài liệu kiến trúc scaling
```

---

## Chạy tests

```bash
go test ./...
```

Tests hiện có: `internal/parser/parser_test.go` — kiểm tra parse, rate limit, context cancellation.

---

## Biến môi trường

| Biến | Mặc định | Mô tả |
|---|---|---|
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | AMQP connection string |
