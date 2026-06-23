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
	mq.Subscribe(ctx, queue.QueueURLs, 1, func(d queue.Delivery) error {
		var env queue.Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			log.Printf("bad message: %v", err)
			return d.Nack(false)
		}
		sp, ok := source.Registry[env.Source]
		if !ok {
			log.Printf("unknown source: %s", env.Source)
			return d.Nack(false)
		}

		record, err := sp.Parse(ctx, env.Payload)

		if err != nil {
			log.Printf("parse error [%s] %s: %v", env.Source, env.Payload, err)
			return d.Nack(true)
		}

		payload, _ := json.Marshal(record.Data)
		body, _ := json.Marshal(queue.Envelope{Source: record.Source, Payload: string(payload)})

		mq.Publish(ctx, queue.QueueData, body)

		log.Printf("parsed [%s] %s", record.Source, env.Payload)

		return d.Ack()
	})
}
