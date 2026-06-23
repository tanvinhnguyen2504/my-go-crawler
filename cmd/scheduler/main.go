package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/my-go-crawler/config"
	"github.com/my-go-crawler/internal/queue"
	"github.com/my-go-crawler/internal/source"
	_ "github.com/my-go-crawler/internal/source/books"
)

func main() {
	rabbitmqUrl := config.EnvOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	mq, err := queue.New(rabbitmqUrl)

	if err != nil {
		log.Fatal(err)
		return
	}
	defer mq.Close()

	if err := mq.DeclareQueue(queue.QueueSeeds); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	count := 0

	for name, sp := range source.Registry {
		for _, seedUrl := range sp.SeedURLs() {
			body, _ := json.Marshal(queue.Envelope{Source: name, Payload: seedUrl})
			if err := mq.Publish(ctx, queue.QueueSeeds, body); err != nil {
				log.Printf("publish error: %v", err)
				continue
			}
			count++
			log.Printf("seeded [%s] %s", name, seedUrl)
		}
	}
	log.Printf("done - seeded %d URLs", count)

}
