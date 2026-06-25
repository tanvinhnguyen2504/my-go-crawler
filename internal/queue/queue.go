package queue

import "context"

const (
	QueueSeeds = "seeds"
	QueueURLs  = "urls"
	QueueData  = "data"
	QueueDead  = "dead"
)

// Envelope là format chuẩn của mọi message trong pipeline.
type Envelope struct {
	Source  string `json:"source"`
	Payload string `json:"payload"`
	Attempt int    `json:"attempt,omitempty"`
}

// Delivery là message nhận được từ queue, kèm hàm Ack/Nack.
// Handler phải gọi Ack sau khi xử lý thành công, hoặc Nack nếu lỗi.
type Delivery struct {
	Body []byte
	Ack  func() error
	Nack func(requeue bool) error
}

type Producer interface {
	Publish(ctx context.Context, queue string, body []byte) error
	Close() error
}

type Consumer interface {
	// DeclareQueue tạo queue nếu chưa có (idempotent, durable).
	DeclareQueue(name string) error
	// Subscribe lắng nghe queue và gọi handler cho mỗi message.
	// prefetch: số message tối đa consumer giữ cùng lúc chưa ACK.
	// Blocking cho đến khi ctx bị cancel hoặc queue bị đóng.
	Subscribe(ctx context.Context, queue string, prefetch int, handler func(Delivery) error) error
	Close() error
}
