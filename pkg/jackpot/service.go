package jackpot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/providers"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const (
	// DefaultBroadcastInterval is the maximum interval for broadcasting buffered updates
	// Actual flush happens when buffer is idle for FlushIdleTimeout
	DefaultBroadcastInterval = 2 * time.Second

	// FlushIdleTimeout is the time to wait after last update before flushing
	// This ensures all updates from the same spin (e.g., 3 pools) are batched together
	FlushIdleTimeout = 300 * time.Millisecond

	// RefreshInterval is the interval for refreshing pool values from the reward provider
	RefreshInterval = 60 * time.Second
)

// Service encapsulates pool registration, contributions, buffering, and broadcasting.
// It is transport-agnostic: caller wires HTTP routes (e.g. /games/{code}/jackpot/updates)
// and subscribes to updates via Listen().
type Service struct {
	mu            sync.RWMutex
	pools         map[string]PoolConfig
	buffer        map[string]map[string]Update // spin_id -> pool_id -> Update (grouped by spin)
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
	
	// Flush timing per spin
	spinTimers        map[string]*time.Timer // spin_id -> timer
	spinTimersMu      sync.Mutex
	spinLastUpdate    map[string]time.Time    // spin_id -> last update time
	spinExpectedPools map[string]int          // spin_id -> expected total pools (0 = unknown)
	flushChan         chan string             // Channel to signal flush for a specific spin_id
}

// NewService creates a new jackpot service.
func NewService(cfg ServiceConfig) *Service {
	interval := cfg.BroadcastInterval
	if interval <= 0 {
		interval = DefaultBroadcastInterval
	}
	s := &Service{
		pools:         make(map[string]PoolConfig),
		buffer:        make(map[string]map[string]Update), // spin_id -> pool_id -> Update
		broad:         NewBroadcaster(128),
		logger:        cfg.Logger,
		interval:      interval,
		stopChan:      make(chan struct{}),
		reward:        cfg.RewardProvider,
		gameCode:      cfg.GameCode,
		spinTimers:        make(map[string]*time.Timer),
		spinLastUpdate:    make(map[string]time.Time),
		spinExpectedPools: make(map[string]int),
		flushChan:         make(chan string, 100), // Buffer for multiple spin flushes
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

// InitializePoolsFromProvider initializes buffer with current pool values from provider.
// This should be called after registering pools to ensure buffer has accurate data from the start.
// If RewardProvider is not set, this method does nothing.
func (s *Service) InitializePoolsFromProvider(ctx context.Context) error {
	s.mu.RLock()
	store := s.reward
	pools := lo.Values(s.pools)
	s.mu.RUnlock()

	if store == nil {
		s.logger.Debug().Msg("RewardProvider not set, skipping pool initialization")
		return nil
	}

	if len(pools) == 0 {
		s.logger.Debug().Msg("No pools registered, skipping initialization")
		return nil
	}

	initializedCount := 0
	for _, pool := range pools {
		poolData, err := store.GetPool(ctx, pool.ID, pool.Init)
		if err != nil {
			s.logger.Debug().
				Err(err).
				Str("pool_id", pool.ID).
				Msg("Failed to initialize pool from provider, using init value")
			// Use init value as fallback - store in a special "init" spin
			s.mu.Lock()
			if s.buffer["init"] == nil {
				s.buffer["init"] = make(map[string]Update)
			}
			s.buffer["init"][pool.ID] = Update{
				PoolID:    pool.ID,
				Amount:    pool.Init,
				Timestamp: time.Now(),
			}
			s.mu.Unlock()
			continue
		}

		// Update buffer with current value from provider - store in a special "init" spin
		s.mu.Lock()
		if s.buffer["init"] == nil {
			s.buffer["init"] = make(map[string]Update)
		}
		s.buffer["init"][pool.ID] = Update{
			PoolID:    pool.ID,
			Amount:    poolData.Amount,
			Timestamp: poolData.UpdatedAt,
		}
		s.mu.Unlock()

		initializedCount++
		s.logger.Debug().
			Str("pool_id", pool.ID).
			Float64("amount", poolData.Amount.InexactFloat64()).
			Msg("Initialized pool from provider")
	}

	s.logger.Info().
		Int("total_pools", len(pools)).
		Int("initialized", initializedCount).
		Msg("Initialized pools from provider")

	return nil
}

// Contribute calculates contributions for a total bet using registered pools.
// prog is treated as percentage (0.01 -> 1% of totalBet).
func (s *Service) Contribute(totalBet decimal.Decimal) []Contribution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return lo.FilterMap(lo.Values(s.pools), func(pool PoolConfig, _ int) (Contribution, bool) {
		amount := totalBet.Mul(pool.Prog)
		if amount.IsZero() {
			return Contribution{}, false
		}
		return Contribution{
			PoolID: pool.ID,
			Amount: amount,
		}, true
	})
}

// ContributeAndStore computes contributions and persists them via a RewardStore (e.g., providers.RewardProvider).
// Note: This method does not generate spin_id. Use ContributeAndStoreWithSpinID if you need spin grouping.
func (s *Service) ContributeAndStore(ctx context.Context, totalBet decimal.Decimal, gameCode, userID string, store RewardProvider) error {
	contribs := s.Contribute(totalBet)
	for _, c := range contribs {
		// No spin_id for legacy calls - reward service will handle grouping
		if err := store.Contribute(ctx, &providers.ContributeRequest{
			PoolID:    c.PoolID,
			UserID:    userID,
			Amount:    c.Amount,
			GameCode:  gameCode,
			SpinID:    "",
			TotalPools: 0,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ContributeAndStoreWithSpinID computes contributions and persists them with spin_id for proper grouping.
func (s *Service) ContributeAndStoreWithSpinID(ctx context.Context, totalBet decimal.Decimal, gameCode, userID, spinID string, store RewardProvider) error {
	contribs := s.Contribute(totalBet)
	totalPools := len(contribs)
	for _, c := range contribs {
		if err := store.Contribute(ctx, &providers.ContributeRequest{
			PoolID:     c.PoolID,
			UserID:     userID,
			Amount:     c.Amount,
			GameCode:   gameCode,
			SpinID:     spinID,
			TotalPools: totalPools,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ContributeAndApply uses the internal RewardStore (if set) and returns the contributions.
// If no RewardStore is configured, it just returns the computed contributions and does not persist.
// Note: This method does not generate spin_id. Use ContributeAndApplyWithSpinID if you need spin grouping.
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
		// No spin_id for legacy calls - reward service will handle grouping
		if err := store.Contribute(ctx, &providers.ContributeRequest{
			PoolID:     c.PoolID,
			UserID:     userID,
			Amount:     c.Amount,
			GameCode:   gameCode,
			SpinID:     "",
			TotalPools: 0,
		}); err != nil {
			return contribs, err
		}
	}
	return contribs, nil
}

// ContributeAndApplyWithSpinID uses the internal RewardStore with spin_id for proper grouping.
func (s *Service) ContributeAndApplyWithSpinID(ctx context.Context, totalBet decimal.Decimal, userID, spinID string) ([]Contribution, error) {
	contribs := s.Contribute(totalBet)
	totalPools := len(contribs)
	s.mu.RLock()
	store := s.reward
	gameCode := s.gameCode
	s.mu.RUnlock()
	if store == nil {
		return contribs, nil
	}
	for _, c := range contribs {
		if err := store.Contribute(ctx, &providers.ContributeRequest{
			PoolID:     c.PoolID,
			UserID:     userID,
			Amount:     c.Amount,
			GameCode:   gameCode,
			SpinID:     spinID,
			TotalPools: totalPools,
		}); err != nil {
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
	if store != nil {
		s.logger.Debug().Msg("RewardProvider set on jackpot service")
	} else {
		s.logger.Warn().Msg("RewardProvider set to nil on jackpot service")
	}
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
	pools := lo.Values(s.pools)
	store := s.reward
	s.mu.RUnlock()

	updates := lo.Map(pools, func(pool PoolConfig, _ int) Update {
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

		return Update{
			PoolID:    pool.ID,
			Amount:    amount,
			Timestamp: updatedAt,
		}
	})

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
	// Flatten buffer: spin_id -> pool_id -> Update -> pool_id -> Update (latest per pool)
	flatBuffer := make(map[string]Update)
	for _, spinUpdates := range s.buffer {
		for poolID, update := range spinUpdates {
			// Keep the latest update for each pool
			if existing, exists := flatBuffer[poolID]; !exists || update.Timestamp.After(existing.Timestamp) {
				flatBuffer[poolID] = update
			}
		}
	}
	registeredPools := lo.Assign(s.pools)
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

		// Always get from provider first (source of truth)
		// Buffer may contain stale or incorrect values from Kafka
		var providerUpdate *Update
		if store != nil {
			poolData, err := store.GetPool(ctx, poolID, initValue)
			if err == nil {
				providerUpdate = &Update{
					PoolID:    poolID,
					Amount:    poolData.Amount,
					Timestamp: poolData.UpdatedAt,
				}
			} else {
				// Log error for debugging
				s.logger.Debug().Err(err).Str("pool_id", poolID).Msg("Failed to get pool from provider")
			}
		} else {
			// RewardProvider is not set - this is a configuration issue
			s.logger.Warn().
				Str("pool_id", poolID).
				Msg("RewardProvider is nil - cannot get accurate pool value from provider. Please call SetRewardProvider() on jackpot service.")
		}

		// Check buffer for potentially newer updates
		bufferedUpdate, hasBuffer := flatBuffer[poolID]

		// Choose the most recent value: compare buffer vs provider
		// Always prefer provider unless buffer is clearly newer (more than a few seconds)
		if hasBuffer && providerUpdate != nil {
			// Both exist: use the one with newer timestamp
			// But only trust buffer if it's significantly newer (at least 1 second)
			// This prevents using stale buffer values
			timeDiff := bufferedUpdate.Timestamp.Sub(providerUpdate.Timestamp)
			if timeDiff > 1*time.Second {
				// Buffer is significantly newer, use it
				s.logger.Debug().
					Str("pool_id", poolID).
					Float64("buffer_amount", bufferedUpdate.Amount.InexactFloat64()).
					Float64("provider_amount", providerUpdate.Amount.InexactFloat64()).
					Dur("time_diff", timeDiff).
					Msg("Using buffer (newer than provider)")
				updates = append(updates, bufferedUpdate)
			} else {
				// Provider is same or newer, use provider (more reliable)
				s.logger.Debug().
					Str("pool_id", poolID).
					Float64("buffer_amount", bufferedUpdate.Amount.InexactFloat64()).
					Float64("provider_amount", providerUpdate.Amount.InexactFloat64()).
					Dur("time_diff", timeDiff).
					Msg("Using provider (same or newer than buffer)")
				updates = append(updates, *providerUpdate)
			}
		} else if providerUpdate != nil {
			// Only provider exists: use provider
			s.logger.Debug().
				Str("pool_id", poolID).
				Float64("provider_amount", providerUpdate.Amount.InexactFloat64()).
				Msg("Using provider (no buffer)")
			updates = append(updates, *providerUpdate)
		} else if hasBuffer {
			// Only buffer exists: use buffer as fallback
			s.logger.Debug().
				Str("pool_id", poolID).
				Float64("buffer_amount", bufferedUpdate.Amount.InexactFloat64()).
				Msg("Using buffer (no provider)")
			updates = append(updates, bufferedUpdate)
		} else {
			// Neither exists: use init value
			s.logger.Debug().
				Str("pool_id", poolID).
				Float64("init_amount", initValue.InexactFloat64()).
				Msg("Using init value (no provider, no buffer)")
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
// Note: Updates from Kafka are buffered and will be broadcast via flush().
// The refresh mechanism will periodically verify and correct buffer values from provider.
// Updates are grouped by spin_id to ensure all pools from the same spin are flushed together.
func (s *Service) HandleKafkaUpdate(update Update) {
	s.mu.Lock()
	// Check if pool belongs to this game (starts with game code)
	if s.gameCode != "" && !strings.HasPrefix(update.PoolID, s.gameCode) {
		// Pool doesn't belong to this game, ignore
		s.mu.Unlock()
		return
	}
	if update.Timestamp.IsZero() {
		update.Timestamp = time.Now()
	}

	// Determine spin_id: use provided spin_id or generate one based on timestamp (for backward compatibility)
	spinID := update.SpinID
	if spinID == "" {
		// If no spin_id provided, use a time-based grouping (every 500ms window)
		// This ensures updates arriving close together are grouped
		spinID = fmt.Sprintf("time_%d", update.Timestamp.UnixMilli()/500)
	}

	// Initialize spin buffer if needed
	if s.buffer[spinID] == nil {
		s.buffer[spinID] = make(map[string]Update)
	}

	// Only update buffer if the new update is newer than existing one
	// This prevents overwriting with stale Kafka messages
	if existingUpdate, exists := s.buffer[spinID][update.PoolID]; exists {
		if update.Timestamp.Before(existingUpdate.Timestamp) {
			// New update is older, ignore it
			s.mu.Unlock()
			s.logger.Debug().
				Str("pool_id", update.PoolID).
				Str("spin_id", spinID).
				Time("existing_timestamp", existingUpdate.Timestamp).
				Time("new_timestamp", update.Timestamp).
				Msg("Ignoring stale Kafka update")
			return
		}
	}

	s.buffer[spinID][update.PoolID] = update
	s.spinLastUpdate[spinID] = time.Now()
	
	// Track expected pools if provided
	if update.TotalPools > 0 {
		s.spinExpectedPools[spinID] = update.TotalPools
	}
	
	currentPoolCount := len(s.buffer[spinID])
	expectedPools := s.spinExpectedPools[spinID]
	wasNewSpin := currentPoolCount == 1 // First update for this spin
	s.mu.Unlock()

	// Check if we have enough pools to flush immediately
	if expectedPools > 0 && currentPoolCount >= expectedPools {
		// We have all expected pools - flush immediately!
		s.logger.Debug().
			Str("spin_id", spinID).
			Int("current_pools", currentPoolCount).
			Int("expected_pools", expectedPools).
			Msg("Received all expected pools, flushing immediately")
		s.flushSpin(spinID)
		return
	}

	// Reset flush timer for this spin - wait for more updates from the same spin
	if wasNewSpin {
		s.resetSpinTimer(spinID)
	} else {
		// Already have updates for this spin, reset timer to wait a bit more
		s.resetSpinTimer(spinID)
	}

	s.logger.Debug().
		Str("pool_id", update.PoolID).
		Str("spin_id", spinID).
		Float64("amount", update.Amount.InexactFloat64()).
		Time("timestamp", update.Timestamp).
		Msg("Buffered Kafka update")
}

// GetRegisteredPoolIDs returns a copy of all registered pool IDs.
// This can be used to create a filter for Kafka consumer.
func (s *Service) GetRegisteredPoolIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return lo.Keys(s.pools)
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
	s.spinTimersMu.Lock()
	for spinID, timer := range s.spinTimers {
		if timer != nil {
			timer.Stop()
		}
		delete(s.spinTimers, spinID)
		delete(s.spinLastUpdate, spinID)
		delete(s.spinExpectedPools, spinID)
	}
	s.spinTimersMu.Unlock()
	close(s.stopChan)
}

// start begins the flush loop and refresh loop.
func (s *Service) start() {
	// Max interval ticker - ensures we flush at least every interval
	s.ticker = time.NewTicker(s.interval)
	s.refreshTicker = time.NewTicker(RefreshInterval)
	
	go s.loop()
	go s.refreshLoop()
}

func (s *Service) loop() {
	for {
		select {
		case <-s.stopChan:
			// Stop all spin timers
			s.spinTimersMu.Lock()
			for spinID, timer := range s.spinTimers {
				if timer != nil {
					timer.Stop()
				}
				delete(s.spinTimers, spinID)
				delete(s.spinLastUpdate, spinID)
				delete(s.spinExpectedPools, spinID)
			}
			s.spinTimersMu.Unlock()
			return
		case <-s.ticker.C:
			// Max interval reached - flush all spins
			s.flushAll()
		case spinID := <-s.flushChan:
			// Spin idle timeout - flush this specific spin
			s.flushSpin(spinID)
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

// resetSpinTimer resets the flush timer for a specific spin_id
func (s *Service) resetSpinTimer(spinID string) {
	s.spinTimersMu.Lock()
	defer s.spinTimersMu.Unlock()
	
	// Stop existing timer if any
	if timer, exists := s.spinTimers[spinID]; exists && timer != nil {
		if !timer.Stop() {
			// Timer already fired, drain the channel
			select {
			case <-timer.C:
			default:
			}
		}
	}
	
	// Create new timer for this spin
	timer := time.NewTimer(FlushIdleTimeout)
	s.spinTimers[spinID] = timer
	
	// Start goroutine to wait for timer and signal flush
	go func(spin string) {
		select {
		case <-timer.C:
			// Timer expired - signal flush for this spin
			select {
			case s.flushChan <- spin:
			default:
				// Channel full, skip (shouldn't happen with buffer size 100)
			}
		}
	}(spinID)
}

// flushSpin flushes all pools for a specific spin_id
func (s *Service) flushSpin(spinID string) {
	s.mu.Lock()
	spinUpdates, exists := s.buffer[spinID]
	if !exists || len(spinUpdates) == 0 {
		s.mu.Unlock()
		// Stop timer for this spin
		s.spinTimersMu.Lock()
		if timer, ok := s.spinTimers[spinID]; ok && timer != nil {
			timer.Stop()
		}
		delete(s.spinTimers, spinID)
		delete(s.spinLastUpdate, spinID)
		delete(s.spinExpectedPools, spinID)
		s.spinTimersMu.Unlock()
		return
	}

	// Get all updates for this spin
	updates := lo.Values(spinUpdates)
	// Remove this spin from buffer
	delete(s.buffer, spinID)
	s.mu.Unlock()

	// Stop timer for this spin
	s.spinTimersMu.Lock()
	if timer, ok := s.spinTimers[spinID]; ok && timer != nil {
		timer.Stop()
	}
	delete(s.spinTimers, spinID)
	delete(s.spinLastUpdate, spinID)
	delete(s.spinExpectedPools, spinID)
	s.spinTimersMu.Unlock()

	// Broadcast all updates for this spin
	for _, u := range updates {
		s.broad.Send(u)
	}
	if s.logger.GetLevel() <= zerolog.DebugLevel {
		s.logger.Debug().
			Str("spin_id", spinID).
			Int("count", len(updates)).
			Msg("flushed jackpot updates for spin")
	}
}

// flushAll flushes all spins (called on max interval)
func (s *Service) flushAll() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}

	// Collect all spins to flush
	spinsToFlush := lo.Keys(s.buffer)
	s.mu.Unlock()

	// Flush each spin
	for _, spinID := range spinsToFlush {
		s.flushSpin(spinID)
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

		// Update buffer with fresh data - check in all spins for this pool
		s.mu.Lock()
		// Find latest update for this pool across all spins
		var latestUpdate *Update
		for _, spinUpdates := range s.buffer {
			if poolUpdate, exists := spinUpdates[poolID]; exists {
				if latestUpdate == nil || poolUpdate.Timestamp.After(latestUpdate.Timestamp) {
					latestUpdate = &poolUpdate
				}
			}
		}
		
		// Only update if provider timestamp is newer than buffer
		if latestUpdate == nil || poolData.UpdatedAt.After(latestUpdate.Timestamp) {
			// Store in refresh spin (or create if doesn't exist)
			if s.buffer["refresh"] == nil {
				s.buffer["refresh"] = make(map[string]Update)
			}
			s.buffer["refresh"][poolID] = update
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
