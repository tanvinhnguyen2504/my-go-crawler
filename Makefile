.PHONY: help build test tidy \
        run run-scheduler run-crawler run-parser run-sink \
        up down up-infra logs

# ─── Default ────────────────────────────────────────────────────────────────

help:
	@echo "Local mode"
	@echo "  make run              Run single-process crawler (writes books.json)"
	@echo ""
	@echo "Distributed mode"
	@echo "  make up-infra         Start RabbitMQ only"
	@echo "  make run-scheduler    Seed the queue (run once)"
	@echo "  make run-crawler      Start crawler service"
	@echo "  make run-parser       Start parser service"
	@echo "  make run-sink         Start sink service"
	@echo "  make up               Start all services via Docker Compose"
	@echo "  make down             Stop all services"
	@echo "  make logs             Tail logs from all services"
	@echo ""
	@echo "Dev"
	@echo "  make build            Build all binaries"
	@echo "  make test             Run all tests"
	@echo "  make tidy             Run go mod tidy"

# ─── Local mode ─────────────────────────────────────────────────────────────

run:
	go run ./main.go

# ─── Distributed mode (local, RabbitMQ must be running) ─────────────────────

up-infra:
	docker compose up rabbitmq -d

run-scheduler:
	go run ./cmd/scheduler

run-crawler:
	go run ./cmd/crawler

run-parser:
	go run ./cmd/parser

run-sink:
	go run ./cmd/sink

# ─── Docker Compose ──────────────────────────────────────────────────────────

up:
	docker compose up --build

# restart:
# 	docker restart my-go

down:
	docker compose down

logs:
	docker compose logs -f

# ─── Dev ────────────────────────────────────────────────────────────────────

build:
	go build ./...

test:
	go test ./...

tidy:
	go mod tidy

up-monitoring:
	docker compose up prometheus grafana -d

open-grafana:
	open http://localhost:3000

open-prometheus:
	open http://localhost:9090/targets

open-rabbitmq:
	open http://localhost:15672