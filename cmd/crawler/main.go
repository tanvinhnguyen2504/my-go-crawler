package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/my-go-crawler/config"
	"github.com/my-go-crawler/internal/queue"
	"github.com/my-go-crawler/internal/source"
)

func main() {
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
		}
		sp, ok := source.Registry[env.Source]
		if !ok {
			log.Printf("unknown source: %s", env.Source)
		}
		urls, err := sp.Crawl(ctx, env.Payload)

		if err != nil {
			log.Printf("crawl error [%s]: %v", env.Source, err)
		}

		for _, url := range urls {
			body, _ := json.Marshal(queue.Envelope{Source: env.Source, Payload: url})
			mq.Publish(ctx, queue.QueueURLs, body)
		}
		// log.Printf("crawled [%s] %s")
		return d.Ack()
	})
}
