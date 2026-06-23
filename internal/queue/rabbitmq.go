package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQ struct {
	conn  *amqp.Connection
	pubCh *amqp.Channel // channel riêng cho publish (thread-safe hơn)
	subCh *amqp.Channel // channel riêng cho consume
}

// New kết nối tới RabbitMQ và mở 2 channel (publish + consume).
// amqpURL ví dụ: "amqp://guest:guest@localhost:5672/"
func New(amqpURL string) (*rabbitMQ, error) {
	conn, err := amqp.Dial(amqpURL)
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

// DeclareQueue tạo queue durable nếu chưa có.
// Idempotent — safe khi gọi nhiều lần hoặc queue đã tồn tại.
func (r *rabbitMQ) DeclareQueue(name string) error {
	_, err := r.subCh.QueueDeclare(name, true, false, false, false, nil)
	return err
}

// Publish ghi một message vào queue qua default exchange.
// DeliveryMode=Persistent đảm bảo message không mất khi RabbitMQ restart.
func (r *rabbitMQ) Publish(ctx context.Context, queue string, body []byte) error {
	return r.pubCh.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// Subscribe lắng nghe queue và gọi handler cho mỗi message.
// Handler tự chịu trách nhiệm gọi Ack hoặc Nack.
// Block cho đến khi ctx bị cancel.
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
