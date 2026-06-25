package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/my-go-crawler/pkg"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQ struct {
	conn  *amqp.Connection
	pubCh *amqp.Channel // channel riêng cho publish (thread-safe hơn)
	subCh *amqp.Channel // channel riêng cho consume
}

func New(amqpURL string) (*rabbitMQ, error) {
	var conn *amqp.Connection
	err := pkg.RetryWithBackoff(
		context.Background(),
		5,
		500*time.Millisecond,
		30*time.Second,
		func(e error) bool { return true },
		func() error {
			var dialErr error
			conn, dialErr = amqp.Dial(amqpURL) // gán vào outer conn, không dùng :=
			if dialErr != nil {
				log.Printf("rabbitmq dial failed: %v — retrying...", dialErr)
			}
			return dialErr
		},
	)
	if err != nil {
		return nil, err
	}
	pubCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	subCh, err := conn.Channel()
	if err != nil {
		pubCh.Close()
		conn.Close()
		return nil, err
	}
	return &rabbitMQ{conn: conn, pubCh: pubCh, subCh: subCh}, nil
}

func (r *rabbitMQ) DeclareQueue(name string) error {
	_, err := r.subCh.QueueDeclare(name, true, false, false, false, nil)
	return err
}

func (r *rabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	return r.pubCh.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

func (r *rabbitMQ) DeadLetter(ctx context.Context, env Envelope, reason string) error {
	env.Payload = fmt.Sprintf(`{"original":%s,"reason":%q}`, mustJSON(env.Payload), reason)
	body, _ := json.Marshal(env)
	return r.Publish(ctx, QueueDead, body)
}

func (r *rabbitMQ) Subscribe(ctx context.Context, queue string, prefetch int, handler func(Delivery) error) error {
	if err := r.subCh.Qos(prefetch, 0, false); err != nil {
		return err
	}
	msgs, err := r.subCh.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			handler(Delivery{
				Body: msg.Body,
				Ack:  func() error { return msg.Ack(false) },
				Nack: func(requeue bool) error { return msg.Nack(false, requeue) },
			})
		}
	}
}

func (r *rabbitMQ) Close() error {
	r.pubCh.Close()
	r.subCh.Close()
	return r.conn.Close()
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
