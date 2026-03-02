package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/derr/pulse/internal/models"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Producer sends enriched RawPost messages to a Kafka topic as JSON.
type Producer struct {
	writer *kafka.Writer
	logger *zap.Logger
}

// NewProducer creates a Kafka writer. brokers is comma-separated (e.g. "localhost:9092" or "broker1:9092,broker2:9092").
// topic defaults to "pulse.posts" if empty. Returns nil if brokers is empty (Kafka disabled).
func NewProducer(brokers, topic string, logger *zap.Logger) (*Producer, error) {
	if brokers == "" {
		return nil, nil
	}
	if topic == "" {
		topic = "pulse.posts"
	}
	addr := kafka.TCP(strings.TrimSpace(strings.Split(brokers, ",")[0]))
	w := &kafka.Writer{
		Addr:         addr,
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		MaxAttempts:  3,
	}
	return &Producer{writer: w, logger: logger}, nil
}

// SendPosts serializes each post as JSON and writes to the topic. Key = post ID. Non-blocking; logs errors.
func (p *Producer) SendPosts(ctx context.Context, posts []models.RawPost) (int, error) {
	if p == nil || len(posts) == 0 {
		return 0, nil
	}
	msgs := make([]kafka.Message, 0, len(posts))
	for _, post := range posts {
		value, err := json.Marshal(post)
		if err != nil {
			p.logger.Warn("kafka marshal post", zap.String("id", post.ID), zap.Error(err))
			continue
		}
		msgs = append(msgs, kafka.Message{
			Key:   []byte(post.ID),
			Value: value,
			Time:  time.Now().UTC(),
		})
	}
	if len(msgs) == 0 {
		return 0, nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
		return 0, err
	}
	p.logger.Info("kafka produced", zap.Int("count", len(msgs)), zap.String("topic", p.writer.Topic))
	return len(msgs), nil
}

// Close closes the Kafka writer.
func (p *Producer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
