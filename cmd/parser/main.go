package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/my-go-crawler/config"
	"github.com/my-go-crawler/internal/metrics"
	"github.com/my-go-crawler/internal/queue"
	"github.com/my-go-crawler/internal/source"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	go metrics.StartServer(":2113")

	fmt.Println("parser...")
	rabbitmqUrl := config.EnvOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	mq, err := queue.New(rabbitmqUrl)

	if err != nil {
		log.Fatal(err)
		return
	}
	defer mq.Close()

	mq.DeclareQueue(queue.QueueURLs)
	mq.DeclareQueue(queue.QueueData)

	log.Println("parser stated, waiting for seed")
	ctx := context.Background()
	mq.Subscribe(ctx, queue.QueueURLs, 5, func(d queue.Delivery) error {
		var env queue.Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			log.Printf("bad message: %v", err)
			return d.Nack(false)
		}

		timer := prometheus.NewTimer(
			metrics.ProcessingDuration.WithLabelValues("parser", env.Source),
		)
		defer timer.ObserveDuration()

		sp, ok := source.Registry[env.Source]
		if !ok {
			log.Printf("unknown source: %s", env.Source)
			metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "error").Inc()
			return d.Nack(false)
		}

		record, err := sp.Parse(ctx, env.Payload)
		if err != nil {
			log.Printf("parse error [%s] %s: %v", env.Source, env.Payload, err)
			metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "error").Inc()
			return d.Nack(true)
		}

		payload, _ := json.Marshal(record.Data)
		body, _ := json.Marshal(queue.Envelope{Source: record.Source, Payload: string(payload)})
		mq.Publish(ctx, queue.QueueData, body)

		log.Printf("parsed [%s] %s", record.Source, env.Payload)
		metrics.MessagesProcessed.WithLabelValues("parser", env.Source, "success").Inc()
		return d.Ack()
	})
}
