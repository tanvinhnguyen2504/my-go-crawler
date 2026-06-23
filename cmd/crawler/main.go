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
	go metrics.StartServer(":2112")

	fmt.Println("crawler..")
	rabbitmqUrl := config.EnvOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	mq, err := queue.New(rabbitmqUrl)

	if err != nil {
		log.Fatal(err)
		return
	}
	defer mq.Close()

	mq.DeclareQueue(queue.QueueSeeds)
	mq.DeclareQueue(queue.QueueURLs)

	log.Println("crawler started, waiting for seed")

	ctx := context.Background()

	mq.Subscribe(ctx, queue.QueueSeeds, 1, func(d queue.Delivery) error {
		var env queue.Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			log.Printf("bad message: %v", err)
			return d.Nack(false)
		}

		timer := prometheus.NewTimer(
			metrics.ProcessingDuration.WithLabelValues("crawler", env.Source),
		)
		defer timer.ObserveDuration()

		sp, ok := source.Registry[env.Source]
		if !ok {
			log.Printf("unknown source: %s", env.Source)
			metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "error").Inc()
			return d.Nack(false)
		}

		urls, err := sp.Crawl(ctx, env.Payload)
		if err != nil {
			log.Printf("crawl error [%s]: %v", env.Source, err)
			metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "error").Inc()
			return d.Nack(true)
		}

		for _, url := range urls {
			body, _ := json.Marshal(queue.Envelope{Source: env.Source, Payload: url})
			mq.Publish(ctx, queue.QueueURLs, body)
		}

		log.Printf("crawled [%s] %s → %d URLs", env.Source, env.Payload, len(urls))
		metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "success").Inc()
		return d.Ack()
	})
}
