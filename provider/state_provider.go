package provider

import (
	"context"
	"encoding/json"
	"fmt"

	coreredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/rs/zerolog"
)

// StateProvider implements server.StateProvider using Redis
type StateProvider struct {
	redis  *coreredis.Client
	logger zerolog.Logger
}

// NewStateProvider creates a new state provider
func NewStateProvider(redisClient *coreredis.Client, logger zerolog.Logger) *StateProvider {
	return &StateProvider{
		redis:  redisClient,
		logger: logger.With().Str("component", "state_provider").Logger(),
	}
}

func (p *StateProvider) stateKey(userID, currencyID, gameCode string) string {
	return fmt.Sprintf("game:state:%s:%s:%s", gameCode, currencyID, userID)
}

// GetPlayerState retrieves player state from Redis
func (p *StateProvider) GetPlayerState(ctx context.Context, userID, currencyID, gameCode string) (interface{}, error) {
	key := p.stateKey(userID, currencyID, gameCode)
	data, err := p.redis.Get(ctx, key)
	if err != nil {
		// Key not found - return default state
		p.logger.Debug().Str("key", key).Msg("No existing state, returning default")
		return game.NewPlayerState(), nil
	}

	var state game.PlayerState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// Initialize ExtraData if nil (for backward compatibility with old states)
	if state.ExtraData == nil {
		state.ExtraData = make(map[string]interface{})
	}

	return &state, nil
}

// SavePlayerState saves player state to Redis
func (p *StateProvider) SavePlayerState(ctx context.Context, userID, currencyID, gameCode string, state interface{}) error {
	key := p.stateKey(userID, currencyID, gameCode)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := p.redis.Set(ctx, key, string(data), 0); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// DeleteState removes player state from Redis
func (p *StateProvider) DeleteState(ctx context.Context, userID, currencyID, gameCode string) error {
	key := p.stateKey(userID, currencyID, gameCode)
	if err := p.redis.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete state: %w", err)
	}
	return nil
}

func (p *StateProvider) spinStateKey(sessionID, gameCode string) string {
	return fmt.Sprintf("game:spin_state:%s:%s", gameCode, sessionID)
}

func (p *StateProvider) GetSpinState(ctx context.Context, sessionID, gameCode string) (interface{}, error) {
	key := p.spinStateKey(sessionID, gameCode)
	data, err := p.redis.Get(ctx, key)
	if err != nil {
		// Key not found - return default state
		p.logger.Debug().Str("key", key).Msg("No existing state, returning default")
		return nil, nil
	}

	var state game.SpinState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

func (p *StateProvider) SaveSpinState(ctx context.Context, sessionID, gameCode string, state interface{}) error {
	key := p.spinStateKey(sessionID, gameCode)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := p.redis.Set(ctx, key, string(data), 0); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

func (p *StateProvider) DeleteSpinState(ctx context.Context, sessionID, gameCode string) error {
	key := p.spinStateKey(sessionID, gameCode)
	if err := p.redis.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete state: %w", err)
	}
	return nil
}
