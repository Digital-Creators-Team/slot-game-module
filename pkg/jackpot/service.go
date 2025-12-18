package jackpot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

const (
	// DefaultBroadcastInterval is the default interval for broadcasting buffered updates
	DefaultBroadcastInterval = 2 * time.Second
	
	// RefreshInterval is the interval for refreshing pool values from the reward provider
	RefreshInterval = 60 * time.Second
)

// Service encapsulates pool registration, contributions, buffering, and broadcasting.
// It is transport-agnostic: caller wires HTTP routes (e.g. /games/{code}/jackpot/updates)
// and subscribes to updates via Listen().
type Service struct {
	mu            sync.RWMutex
	pools         map[string]PoolConfig
	buffer        map[string]Update
	broad         *Broadcaster
	logger        zerolog.Logger
	interval      time.Duration
	ticker        *time.Ticker
	refreshTicker *time.Ticker
	stopChan      chan struct{}
	reward        RewardProvider
	gameCode      string
	refreshing    bool // Flag to prevent concurrent refreshes
	refreshMu     sync.Mutex
}

// NewService creates a new jackpot service.
func NewService(cfg ServiceConfig) *Service {
	interval := cfg.BroadcastInterval
	if interval <= 0 {
		interval = DefaultBroadcastInterval
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

// GetPoolsByIDs returns the current state of pools by their IDs.
// This method checks:
// 1. Buffer (most recent update if available)
// 2. Registered pools + reward provider
// 3. Reward provider directly (if pool not registered but initValue is provided)
// If initValueGetter is provided, it will be used to get init values for unregistered pools.
// initValueGetter signature: func(poolID string) (decimal.Decimal, error)
func (s *Service) GetPoolsByIDs(ctx context.Context, poolIDs []string, initValueGetter func(poolID string) (decimal.Decimal, error)) ([]Update, error) {
	s.mu.RLock()
	buffer := make(map[string]Update)
	for k, v := range s.buffer {
		buffer[k] = v
	}
	registeredPools := make(map[string]PoolConfig)
	for k, v := range s.pools {
		registeredPools[k] = v
	}
	store := s.reward
	s.mu.RUnlock()

	updates := make([]Update, 0, len(poolIDs))
	for _, poolID := range poolIDs {
		var initValue decimal.Decimal

		// Get init value first (needed for reward provider)
		if pool, ok := registeredPools[poolID]; ok {
			initValue = pool.Init
		} else if initValueGetter != nil {
			// Try to get init value from game module
			var err error
			initValue, err = initValueGetter(poolID)
			if err != nil {
				// If we can't get init value, skip this pool
				continue
			}
		} else {
			// No init value available, skip this pool
			continue
		}

		// Check buffer first (most efficient - no provider call needed)
		bufferedUpdate, hasBuffer := buffer[poolID]
		
		// If buffer exists and is recent (within refresh interval), use it directly
		// This avoids unnecessary provider calls since we have periodic refresh
		if hasBuffer {
			age := time.Since(bufferedUpdate.Timestamp)
			if age < RefreshInterval {
				// Buffer is fresh, use it
				updates = append(updates, bufferedUpdate)
				continue
			}
			// Buffer exists but is stale, will check provider below
		}

		// Buffer doesn't exist or is stale: get from provider
		var providerUpdate *Update
		if store != nil {
			poolData, err := store.GetPool(ctx, poolID, initValue)
			if err == nil {
				providerUpdate = &Update{
					PoolID:    poolID,
					Amount:    poolData.Amount,
					Timestamp: poolData.UpdatedAt,
				}
			}
		}

		// Choose the most recent value: compare buffer vs provider
		if hasBuffer && providerUpdate != nil {
			// Both exist: use the one with newer timestamp
			if bufferedUpdate.Timestamp.After(providerUpdate.Timestamp) {
				updates = append(updates, bufferedUpdate)
			} else {
				updates = append(updates, *providerUpdate)
			}
		} else if hasBuffer {
			// Only buffer exists (but stale): use buffer anyway
			updates = append(updates, bufferedUpdate)
		} else if providerUpdate != nil {
			// Only provider exists: use provider
			updates = append(updates, *providerUpdate)
		} else {
			// Neither exists: use init value
			updates = append(updates, Update{
				PoolID:    poolID,
				Amount:    initValue,
				Timestamp: time.Now(),
			})
		}
	}

	return updates, nil
}

// HandleKafkaUpdate buffers an external update (e.g. from Kafka).
// Caller provides poolID + new amount. This is transport-agnostic.
// Pools are created dynamically (pool_id includes bet_multiplier), so we don't require pre-registration.
// We only check if pool_id starts with game code to filter pools belonging to this game.
func (s *Service) HandleKafkaUpdate(update Update) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check if pool belongs to this game (starts with game code)
	if s.gameCode != "" && !strings.HasPrefix(update.PoolID, s.gameCode) {
		// Pool doesn't belong to this game, ignore
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

// CreatePoolFilter creates a filter function that returns true only for pools belonging to this game.
// This can be used with kafka.Consumer.SetPoolFilter() to skip pools not belonging to this game.
// Pools are created dynamically (pool_id includes bet_multiplier), so we only check game code prefix.
func (s *Service) CreatePoolFilter() func(poolID string) bool {
	return func(poolID string) bool {
		s.mu.RLock()
		defer s.mu.RUnlock()

		// If game code is set, check if pool_id starts with game code
		if s.gameCode != "" {
			return strings.HasPrefix(poolID, s.gameCode)
		}

		// If no game code, accept all pools (backward compatibility)
		return true
	}
}

// Listen returns a channel to receive flushed updates plus a cancel function.
func (s *Service) Listen(ctx context.Context) (<-chan Update, context.CancelFunc) {
	return s.broad.Listen(ctx)
}

// Stop stops the service tickers.
func (s *Service) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.refreshTicker != nil {
		s.refreshTicker.Stop()
	}
	close(s.stopChan)
}

// start begins the flush loop and refresh loop.
func (s *Service) start() {
	s.ticker = time.NewTicker(s.interval)
	s.refreshTicker = time.NewTicker(RefreshInterval)
	go s.loop()
	go s.refreshLoop()
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

func (s *Service) refreshLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.refreshTicker.C:
			s.refreshPoolsFromProvider(context.Background())
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

// refreshPoolsFromProvider periodically refreshes pool values from the reward provider.
// This ensures clients get accurate values even if Kafka updates are delayed or missing.
// Only refreshes registered pools (pools with known init values).
// This method is safe to call concurrently - it will skip if a refresh is already in progress.
func (s *Service) refreshPoolsFromProvider(ctx context.Context) {
	// Prevent concurrent refreshes
	s.refreshMu.Lock()
	if s.refreshing {
		s.refreshMu.Unlock()
		return
	}
	s.refreshing = true
	s.refreshMu.Unlock()

	defer func() {
		s.refreshMu.Lock()
		s.refreshing = false
		s.refreshMu.Unlock()
	}()

	s.mu.RLock()
	// Collect registered pools with their init values
	poolIDsToRefresh := make(map[string]decimal.Decimal, len(s.pools))
	for poolID, pool := range s.pools {
		poolIDsToRefresh[poolID] = pool.Init
	}
	store := s.reward
	s.mu.RUnlock()

	if store == nil {
		// No reward provider, nothing to refresh
		return
	}

	if len(poolIDsToRefresh) == 0 {
		// No registered pools to refresh
		return
	}

	// Refresh each pool from provider
	refreshedCount := 0
	for poolID, initValue := range poolIDsToRefresh {
		poolData, err := store.GetPool(ctx, poolID, initValue)
		if err != nil {
			// Log error but continue with other pools
			s.logger.Debug().Err(err).Str("pool_id", poolID).Msg("Failed to refresh pool from provider")
			continue
		}

		// Create update from provider data
		update := Update{
			PoolID:    poolID,
			Amount:    poolData.Amount,
			Timestamp: poolData.UpdatedAt,
		}

		// Update buffer with fresh data
		s.mu.Lock()
		// Only update if provider timestamp is newer than buffer
		if existingUpdate, exists := s.buffer[poolID]; !exists || poolData.UpdatedAt.After(existingUpdate.Timestamp) {
			s.buffer[poolID] = update
			refreshedCount++
		}
		s.mu.Unlock()

		// Broadcast the refreshed update immediately
		s.broad.Send(update)
	}

	if refreshedCount > 0 {
		s.logger.Debug().Int("count", refreshedCount).Msg("refreshed pools from provider")
	}
}
