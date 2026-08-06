package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

const defaultWorkerNum = 10

// Producer wraps Kafka producer functionality
type Producer struct {
	writer    *kafka.Writer
	logger    zerolog.Logger
	jobs      chan kafka.Message
	workerNum int
	wg        sync.WaitGroup
}

// ProducerConfig holds configuration for Kafka producer
type ProducerConfig struct {
	Brokers   []string
	Logger    zerolog.Logger
	WorkerNum int
}

// NewProducer creates a new Kafka producer from brokers list (convenience function)
func NewProducer(brokers []string) (*Producer, error) {
	if len(brokers) == 0 {
		return nil, nil
	}
	return NewProducerWithConfig(ProducerConfig{Brokers: brokers})
}

// NewProducerWithConfig creates a new Kafka producer with full config
func NewProducerWithConfig(config ProducerConfig) (*Producer, error) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		MaxAttempts:  3,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
		Async:        false,
	}

	workerNum := config.WorkerNum
	if workerNum <= 0 {
		workerNum = defaultWorkerNum
	}

	p := &Producer{
		writer:    writer,
		logger:    config.Logger.With().Str("component", "kafka-producer").Logger(),
		jobs:      make(chan kafka.Message, 100),
		workerNum: workerNum,
	}

	// Start workers
	for i := 0; i < workerNum; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	return p, nil
}

func (p *Producer) worker() {
	defer p.wg.Done()
	for msg := range p.jobs {
		func() {
			defer p.recover()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := p.writer.WriteMessages(ctx, msg); err != nil {
				p.logger.Error().
					Err(err).
					Str("topic", msg.Topic).
					Str("key", string(msg.Key)).
					Msg("Failed to send message to Kafka")
			} else {
				p.logger.Debug().
					Str("topic", msg.Topic).
					Str("key", string(msg.Key)).
					Msg("Message sent to Kafka")
			}
		}()
	}
}

// SendMessage sends a message to a Kafka topic (async via worker pool)
func (p *Producer) SendMessage(topic string, key string, value interface{}) error {
	eventBytes, err := json.Marshal(value)
	if err != nil {
		p.logger.Error().Err(err).Msg("Failed to marshal event")
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	p.jobs <- kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: eventBytes,
		Time:  time.Now(),
	}
	return nil
}

// SendMessageSync sends a message synchronously
func (p *Producer) SendMessageSync(ctx context.Context, topic string, key string, value interface{}) error {
	eventBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: eventBytes,
		Time:  time.Now(),
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		p.logger.Error().
			Err(err).
			Str("topic", topic).
			Str("key", key).
			Msg("Failed to send message to Kafka")
		return err
	}

	p.logger.Debug().
		Str("topic", topic).
		Str("key", key).
		Msg("Message sent to Kafka")

	return nil
}

// SendBatch sends multiple messages to Kafka
func (p *Producer) SendBatch(ctx context.Context, messages []kafka.Message) error {
	if err := p.writer.WriteMessages(ctx, messages...); err != nil {
		p.logger.Error().
			Err(err).
			Int("count", len(messages)).
			Msg("Failed to send batch to Kafka")
		return err
	}

	p.logger.Debug().
		Int("count", len(messages)).
		Msg("Batch sent to Kafka")

	return nil
}

// Close closes the Kafka producer
func (p *Producer) Close() error {
	close(p.jobs)
	p.wg.Wait()
	if err := p.writer.Close(); err != nil {
		p.logger.Error().Err(err).Msg("Error closing Kafka producer")
		return err
	}
	return nil
}

func (p *Producer) recover() {
	if r := recover(); r != nil {
		stack := debug.Stack()
		p.logger.Error().
			Str("operation", "send_message_kafka").
			Str("panic", fmt.Sprintf("%v", r)).
			Str("stack_trace", string(stack)).
			Msg("Panic recovered")
	}
}


