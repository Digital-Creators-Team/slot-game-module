package kafka

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

// Event represents a generic Kafka event
type Event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// PoolUpdateEvent represents a jackpot pool update event
type PoolUpdateEvent struct {
	PoolID    string          `json:"pool_id"`
	Amount    decimal.Decimal `json:"amount"`
	GameID    string          `json:"game_id,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// PoolCache is an in-memory cache for pool amounts
type PoolCache struct {
	mu     sync.RWMutex
	pools  map[string]decimal.Decimal
	logger zerolog.Logger
}

const allPoolsKey = "*"

// NewPoolCache creates a new pool cache
func NewPoolCache(logger zerolog.Logger) *PoolCache {
	return &PoolCache{
		pools:  make(map[string]decimal.Decimal),
		logger: logger,
	}
}

// Get retrieves a pool amount from cache
func (pc *PoolCache) Get(poolID string) (decimal.Decimal, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	amount, exists := pc.pools[poolID]
	return amount, exists
}

// Set updates a pool amount in cache
func (pc *PoolCache) Set(poolID string, amount decimal.Decimal) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.pools[poolID] = amount
	pc.logger.Debug().
		Str("pool_id", poolID).
		Str("amount", amount.String()).
		Msg("Pool cache updated")
}

// Subscription represents a client subscription for events
type Subscription struct {
	ID      string
	PoolID  string
	Channel chan PoolUpdateEvent
}

// Consumer represents a Kafka consumer
type Consumer struct {
	reader    *kafka.Reader
	poolCache *PoolCache
	logger    zerolog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	mu          sync.RWMutex
	subscribers map[string][]*Subscription
}

// ConsumerConfig holds Kafka consumer configuration
type ConsumerConfig struct {
	Brokers       []string
	Topic         string
	ConsumerGroup string
	Logger        zerolog.Logger
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(config ConsumerConfig, poolCache *PoolCache) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.ConsumerGroup,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader:      reader,
		poolCache:   poolCache,
		logger:      config.Logger,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[string][]*Subscription),
	}
}

// Start begins consuming messages
func (c *Consumer) Start() error {
	c.wg.Add(1)
	go c.consume()
	c.logger.Info().Msg("Kafka consumer started")
	return nil
}

// Stop gracefully stops the consumer
func (c *Consumer) Stop() error {
	c.logger.Info().Msg("Stopping Kafka consumer...")
	c.cancel()
	c.wg.Wait()

	if err := c.reader.Close(); err != nil {
		c.logger.Error().Err(err).Msg("Error closing Kafka reader")
		return err
	}

	c.logger.Info().Msg("Kafka consumer stopped")
	return nil
}

// consume is the main consumer loop
func (c *Consumer) consume() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			msg, err := c.reader.FetchMessage(c.ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				c.logger.Error().Err(err).Msg("Error fetching message from Kafka")
				time.Sleep(time.Second)
				continue
			}

			if err := c.handleMessage(msg); err != nil {
				c.logger.Error().
					Err(err).
					Str("topic", msg.Topic).
					Int("partition", msg.Partition).
					Int64("offset", msg.Offset).
					Msg("Error handling message")
			}

			if err := c.reader.CommitMessages(c.ctx, msg); err != nil {
				c.logger.Error().Err(err).Msg("Error committing message")
			}
		}
	}
}

// handleMessage processes a single Kafka message
func (c *Consumer) handleMessage(msg kafka.Message) error {
	c.logger.Debug().
		Str("topic", msg.Topic).
		Int("partition", msg.Partition).
		Int64("offset", msg.Offset).
		Msg("Received message")

	var event PoolUpdateEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return err
	}

	// Update cache
	c.poolCache.Set(event.PoolID, event.Amount)

	// Broadcast to subscribers
	c.mu.RLock()
	if subs, exists := c.subscribers[event.PoolID]; exists {
		for _, sub := range subs {
			select {
			case sub.Channel <- event:
			default:
				c.logger.Warn().
					Str("sub_id", sub.ID).
					Str("pool_id", event.PoolID).
					Msg("Subscriber channel full, dropping event")
			}
		}
	}
	// Broadcast to wildcard subscribers
	if subs, exists := c.subscribers[allPoolsKey]; exists {
		for _, sub := range subs {
			select {
			case sub.Channel <- event:
			default:
				c.logger.Warn().
					Str("sub_id", sub.ID).
					Str("pool_id", event.PoolID).
					Msg("Subscriber channel full, dropping event")
			}
		}
	}
	c.mu.RUnlock()

	c.logger.Info().
		Str("pool_id", event.PoolID).
		Str("amount", event.Amount.String()).
		Msg("Pool update processed")

	return nil
}

// GetPoolCache returns the pool cache
func (c *Consumer) GetPoolCache() *PoolCache {
	return c.poolCache
}

// Subscribe subscribes to pool updates for a specific pool ID
func (c *Consumer) Subscribe(poolID string) *Subscription {
	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &Subscription{
		ID:      uuid.New().String(),
		PoolID:  poolID,
		Channel: make(chan PoolUpdateEvent, 10),
	}

	if _, exists := c.subscribers[poolID]; !exists {
		c.subscribers[poolID] = make([]*Subscription, 0)
	}
	c.subscribers[poolID] = append(c.subscribers[poolID], sub)

	c.logger.Debug().
		Str("pool_id", poolID).
		Str("sub_id", sub.ID).
		Msg("New subscription added")

	return sub
}

// SubscribeAll subscribes to all pool updates.
func (c *Consumer) SubscribeAll() *Subscription {
	return c.Subscribe(allPoolsKey)
}

// Unsubscribe removes a subscription
func (c *Consumer) Unsubscribe(sub *Subscription) {
	c.UnsubscribeWithPoolID(sub.PoolID, sub.ID)
}

// UnsubscribeWithPoolID removes a subscription knowing the poolID
func (c *Consumer) UnsubscribeWithPoolID(poolID string, subID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	subs, exists := c.subscribers[poolID]
	if !exists {
		return
	}

	newSubs := make([]*Subscription, 0, len(subs))
	for _, s := range subs {
		if s.ID == subID {
			close(s.Channel)
			continue
		}
		newSubs = append(newSubs, s)
	}

	if len(newSubs) == 0 {
		delete(c.subscribers, poolID)
	} else {
		c.subscribers[poolID] = newSubs
	}

	c.logger.Debug().
		Str("pool_id", poolID).
		Str("sub_id", subID).
		Msg("Subscription removed")
}


