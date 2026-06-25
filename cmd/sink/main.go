package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/my-go-crawler/config"
	"github.com/my-go-crawler/internal/metrics"
	"github.com/my-go-crawler/internal/queue"
	_ "github.com/my-go-crawler/internal/source/books"
	"github.com/my-go-crawler/pkg"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	go metrics.StartServer(":2114")

	rabbitmqUrl := config.EnvOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	mq, err := queue.New(rabbitmqUrl)

	if err != nil {
		log.Fatal(err)
	}
	defer mq.Close()

	mq.DeclareQueue(queue.QueueData)

	ctx := context.Background()

	mq.Subscribe(ctx, queue.QueueData, 10, func(d queue.Delivery) error {
		var env queue.Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			return d.Nack(false)
		}

		timer := prometheus.NewTimer(
			metrics.ProcessingDuration.WithLabelValues("sink", env.Source),
		)
		defer timer.ObserveDuration()

		var data map[string]any
		json.Unmarshal([]byte(env.Payload), &data)

		switch env.Source {
		case "books":
			fmt.Println("[JSON]")
			pkg.DebugJson(data)
		default:
			log.Printf("[unknown source: %s]", env.Source)
		}

		metrics.MessagesProcessed.WithLabelValues("sink", env.Source, "success").Inc()
		return d.Ack()
	})
}
