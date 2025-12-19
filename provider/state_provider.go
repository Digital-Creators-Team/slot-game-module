package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreredis "git.futuregamestudio.net/be-shared/slot-game-module.git/db/redis"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
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

func (p *StateProvider) stateKey(userID, gameCode string) string {
	return fmt.Sprintf("game:state:%s:%s", gameCode, userID)
}

// GetPlayerState retrieves player state from Redis
func (p *StateProvider) GetPlayerState(ctx context.Context, userID, gameCode string) (interface{}, error) {
	key := p.stateKey(userID, gameCode)
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
func (p *StateProvider) SavePlayerState(ctx context.Context, userID, gameCode string, state interface{}) error {
	key := p.stateKey(userID, gameCode)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// State expires after 24 hours
	if err := p.redis.Set(ctx, key, string(data), 24*time.Hour); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// DeleteState removes player state from Redis
func (p *StateProvider) DeleteState(ctx context.Context, userID, gameCode string) error {
	key := p.stateKey(userID, gameCode)
	if err := p.redis.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete state: %w", err)
	}
	return nil
}
