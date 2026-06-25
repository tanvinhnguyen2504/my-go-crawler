package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/my-go-crawler/config"
	"github.com/my-go-crawler/internal/metrics"
	"github.com/my-go-crawler/internal/queue"
	"github.com/my-go-crawler/internal/source"
	_ "github.com/my-go-crawler/internal/source/books"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	maxMessageAttempts = 5
	baseRetryDelay     = 2 * time.Second
	maxRetryDelay      = 60 * time.Second
)

func main() {
	go metrics.StartServer(":2112")

	rabbitmqUrl := config.EnvOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	mq, err := queue.New(rabbitmqUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer mq.Close()

	mq.DeclareQueue(queue.QueueSeeds)
	mq.DeclareQueue(queue.QueueURLs)
	mq.DeclareQueue(queue.QueueDead)

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
			log.Printf("crawl error [%s] attempt %d: %v", env.Source, env.Attempt+1, err)
			metrics.MessagesProcessed.WithLabelValues("crawler", env.Source, "error").Inc()

			env.Attempt++
			if env.Attempt >= maxMessageAttempts {
				log.Printf("dead-lettering [%s] %s after %d attempts", env.Source, env.Payload, env.Attempt)
				if dlErr := mq.DeadLetter(ctx, env, err.Error()); dlErr != nil {
					log.Printf("dead-letter failed: %v — dropping", dlErr)
				}
				return d.Ack()
			}

			delay := baseRetryDelay * (1 << (env.Attempt - 1))
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			log.Printf("retry [%s] attempt %d/%d in %v", env.Source, env.Attempt, maxMessageAttempts, delay)
			time.Sleep(delay)
			body, _ := json.Marshal(env)
			if pubErr := mq.Publish(ctx, queue.QueueSeeds, body); pubErr != nil {
				log.Printf("re-publish failed: %v", pubErr)
				return d.Nack(false)
			}
			return d.Ack()
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
