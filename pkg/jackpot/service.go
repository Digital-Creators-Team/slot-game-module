package jackpot

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// Service encapsulates pool registration, contributions, buffering, and broadcasting.
// It is transport-agnostic: caller wires HTTP routes (e.g. /games/{code}/jackpot/updates)
// and subscribes to updates via Listen().
type Service struct {
	mu       sync.RWMutex
	pools    map[string]PoolConfig
	buffer   map[string]Update
	broad    *Broadcaster
	logger   zerolog.Logger
	interval time.Duration
	ticker   *time.Ticker
	stopChan chan struct{}
	reward   RewardProvider
	gameCode string
}

// NewService creates a new jackpot service.
func NewService(cfg ServiceConfig) *Service {
	interval := cfg.BroadcastInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	s := &Service{
		pools:    make(map[string]PoolConfig),
		buffer:   make(map[string]Update),
		broad:    NewBroadcaster(128),
		logger:   cfg.Logger,
		interval: interval,
		stopChan: make(chan struct{}),
		reward:   cfg.RewardProvider,
		gameCode: cfg.GameCode,
	}
	s.start()
	return s
}

// RegisterPool registers a pool with init value and progressive rate.
func (s *Service) RegisterPool(cfg PoolConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pools[cfg.ID] = cfg
}

// Contribute calculates contributions for a total bet using registered pools.
// prog is treated as percentage (0.01 -> 1% of totalBet).
func (s *Service) Contribute(totalBet decimal.Decimal) []Contribution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Contribution, 0, len(s.pools))
	for _, pool := range s.pools {
		amount := totalBet.Mul(pool.Prog)
		if amount.IsZero() {
			continue
		}
		out = append(out, Contribution{
			PoolID: pool.ID,
			Amount: amount,
		})
	}
	return out
}

// ContributeAndStore computes contributions and persists them via a RewardStore (e.g., providers.RewardProvider).
func (s *Service) ContributeAndStore(ctx context.Context, totalBet decimal.Decimal, gameCode, userID string, store RewardProvider) error {
	contribs := s.Contribute(totalBet)
	for _, c := range contribs {
		if err := store.Contribute(ctx, c.PoolID, userID, c.Amount, gameCode); err != nil {
			return err
		}
	}
	return nil
}

// ContributeAndApply uses the internal RewardStore (if set) and returns the contributions.
// If no RewardStore is configured, it just returns the computed contributions and does not persist.
func (s *Service) ContributeAndApply(ctx context.Context, totalBet decimal.Decimal, userID string) ([]Contribution, error) {
	contribs := s.Contribute(totalBet)
	s.mu.RLock()
	store := s.reward
	gameCode := s.gameCode
	s.mu.RUnlock()
	if store == nil {
		return contribs, nil
	}
	for _, c := range contribs {
		if err := store.Contribute(ctx, c.PoolID, userID, c.Amount, gameCode); err != nil {
			return contribs, err
		}
	}
	return contribs, nil
}

// SetRewardProvider sets the reward provider to persist contributions.
func (s *Service) SetRewardProvider(store RewardProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reward = store
}

// SetGameCode sets the default game code used for persisting contributions.
func (s *Service) SetGameCode(gameCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gameCode = gameCode
}

// RewardProvider returns the configured reward provider (may be nil).
func (s *Service) RewardProvider() RewardProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reward
}

// GetCurrentPools returns the current state of all registered pools.
// If RewardProvider is configured, it fetches current amounts from the provider.
// Otherwise, it returns pools with their init values.
func (s *Service) GetCurrentPools(ctx context.Context) ([]Update, error) {
	s.mu.RLock()
	pools := make([]PoolConfig, 0, len(s.pools))
	for _, pool := range s.pools {
		pools = append(pools, pool)
	}
	store := s.reward
	s.mu.RUnlock()

	updates := make([]Update, 0, len(pools))
	for _, pool := range pools {
		var amount decimal.Decimal
		var updatedAt time.Time

		if store != nil {
			// Try to get current amount from provider
			poolData, err := store.GetPool(ctx, pool.ID, pool.Init)
			if err != nil {
				// If error, fallback to init value
				amount = pool.Init
				updatedAt = time.Now()
			} else {
				amount = poolData.Amount
				updatedAt = poolData.UpdatedAt
			}
		} else {
			// No provider, use init value
			amount = pool.Init
			updatedAt = time.Now()
		}

		updates = append(updates, Update{
			PoolID:    pool.ID,
			Amount:    amount,
			Timestamp: updatedAt,
		})
	}

	return updates, nil
}

// HandleKafkaUpdate buffers an external update (e.g. from Kafka).
// Caller provides poolID + new amount. This is transport-agnostic.
func (s *Service) HandleKafkaUpdate(update Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only buffer updates for registered pools
	if _, exists := s.pools[update.PoolID]; !exists {
		// Pool not registered, ignore this update
		return
	}
	if update.Timestamp.IsZero() {
		update.Timestamp = time.Now()
	}
	s.buffer[update.PoolID] = update
}

// GetRegisteredPoolIDs returns a copy of all registered pool IDs.
// This can be used to create a filter for Kafka consumer.
func (s *Service) GetRegisteredPoolIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	poolIDs := make([]string, 0, len(s.pools))
	for poolID := range s.pools {
		poolIDs = append(poolIDs, poolID)
	}
	return poolIDs
}

// CreatePoolFilter creates a filter function that returns true only for registered pools.
// This can be used with kafka.Consumer.SetPoolFilter() to skip pools not belonging to this game.
func (s *Service) CreatePoolFilter() func(poolID string) bool {
	return func(poolID string) bool {
		s.mu.RLock()
		defer s.mu.RUnlock()
		_, exists := s.pools[poolID]
		return exists
	}
}

// Listen returns a channel to receive flushed updates plus a cancel function.
func (s *Service) Listen(ctx context.Context) (<-chan Update, context.CancelFunc) {
	return s.broad.Listen(ctx)
}

// Stop stops the service ticker.
func (s *Service) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
}

// start begins the flush loop.
func (s *Service) start() {
	s.ticker = time.NewTicker(s.interval)
	go s.loop()
}

func (s *Service) loop() {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ticker.C:
			s.flush()
		}
	}
}

// flush broadcasts buffered updates and clears buffer.
func (s *Service) flush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}

	updates := make([]Update, 0, len(s.buffer))
	for _, u := range s.buffer {
		updates = append(updates, u)
	}
	s.buffer = make(map[string]Update)
	s.mu.Unlock()

	for _, u := range updates {
		s.broad.Send(u)
	}
	if s.logger.GetLevel() <= zerolog.DebugLevel {
		s.logger.Debug().Int("count", len(updates)).Msg("flushed jackpot updates")
	}
}
