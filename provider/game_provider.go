package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	coreredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/cache"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

// gameProvider implements server.GameProvider using HTTP client
type gameProvider struct {
	gameCode             string
	baseURL              string
	httpClient           *http.Client
	cacheTTL             time.Duration
	gameMap              cache.Cache[server.TenantGame]
	disableGameCallbacks []func(context.Context, server.TenantGame)
	logger               logging.LoggerProvider
}

// NewGameProvider creates a new game provider
func NewGameProvider(
	cfg *config.Config,
	logger zerolog.Logger,
	redisClient *coreredis.Client,
) server.GameProvider {
	gameConfig := cfg.ExternalServices.GameService
	timeout := cfg.ExternalServices.WalletService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	cacheTTL := gameConfig.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	p := &gameProvider{
		baseURL: gameConfig.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL:             cacheTTL,
		gameMap:              cache.NewTTLMap[server.TenantGame](),
		disableGameCallbacks: []func(context.Context, server.TenantGame){},
		logger: logging.NewLoggerProvider(logger.With().
			Str("component", "game_provider").
			Logger(),
		),
	}

	if redisClient != nil && len(gameConfig.EventChannel) > 0 {
		go p.subscribeGameEvent(redisClient, gameConfig.EventChannel)
	}

	return p
}

func (p *gameProvider) SetGameCode(code string) {
	p.gameCode = code
	p.logger = p.logger.With(func(loggerContext zerolog.Context) zerolog.Context {
		return loggerContext.Str("game_code", code)
	})
}

func (p *gameProvider) Get(ctx context.Context, tenantID string, skipCache bool) (*server.TenantGame, error) {
	if !skipCache {
		cached, err := p.gameMap.Get(ctx, tenantID)
		if err == nil {
			return &cached, nil
		}
		p.logger.For(ctx).Warn().
			Err(err).
			Str("tenant_id", tenantID).
			Msg("game cache miss")
	}

	game, err := p.get(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if game == nil {
		return nil, server.ErrTenantGameNotFound
	}

	err = p.gameMap.Set(ctx, tenantID, *game, p.cacheTTL)
	if err != nil {
		p.logger.For(ctx).Warn().
			Err(err).
			Str("tenant_id", tenantID).
			Msg("failed to set game cache")
	}
	return game, nil
}

func (p *gameProvider) get(ctx context.Context, tenantID string) (*server.TenantGame, error) {
	url := fmt.Sprintf("%s/api/v2/game/get/%s/%s", p.baseURL, p.gameCode, tenantID)

	req, err := utils.MakeRequest[any](ctx, p.logger.For(ctx), url, nil)
	if err != nil {
		return nil, err
	}

	respData, err := utils.DoInternalRequest[server.TenantGame](p.logger.For(ctx), p.httpClient, req)
	if err != nil {
		return nil, err
	}

	return &respData.Data, nil
}

func (p *gameProvider) AddDisableGameCallback(ctx context.Context, callback func(context.Context, server.TenantGame)) {
	p.disableGameCallbacks = append(p.disableGameCallbacks, callback)
}

func (p *gameProvider) runDisableGameCallbacks(ctx context.Context, game server.TenantGame) {
	for _, callback := range p.disableGameCallbacks {
		callback(ctx, game)
	}
}

type gameEvent struct {
	Timestamp time.Time       `json:"timestamp"`
	GameCode  string          `json:"game_code"`
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	Details   json.RawMessage `json:"details"`
}

func (p *gameProvider) subscribeGameEvent(redisClient *coreredis.Client, eventChannel string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps := redisClient.GetClient().Subscribe(ctx, eventChannel)
	defer func(ps *redis.PubSub) {
		err := ps.Close()
		if err != nil {
			p.logger.For(ctx).Error().Err(err).Msg("failed to close game event redis subscription")
		}
	}(ps)

	ch := ps.Channel()

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-ch:
			if msg == nil {
				p.logger.For(ctx).Error().
					Msg("nil message received")
				continue
			}

			var event gameEvent

			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				p.logger.For(ctx).Error().
					Err(err).
					Msg("failed to parse game refresh event")

				continue
			}

			switch event.Type {
			case "update":
				break
			default:
				continue
			}

			if event.GameCode != p.gameCode {
				continue
			}

			cached, err := p.gameMap.Get(ctx, event.TenantID)
			if errors.Is(err, server.ErrTenantGameNotFound) {
				continue
			}

			if cached.GameCode == "" {
				err = p.gameMap.Delete(ctx, event.TenantID)
				if err != nil {
					p.logger.For(ctx).Error().
						Err(err).
						Str("tenant_id", event.TenantID).
						Msg("failed to clear game cache")
				}

				continue
			}

			var updated server.TenantGame
			err = json.Unmarshal(event.Details, &updated)
			if err != nil {
				p.logger.For(ctx).Error().
					Err(err).
					Any("details", event.Details).
					Msg("Failed to unmarshal event details")

				err = p.gameMap.Delete(ctx, event.TenantID)
				if err != nil {
					p.logger.For(ctx).Error().
						Err(err).
						Str("tenant_id", event.TenantID).
						Msg("failed to clear game cache")
				}

				continue
			}

			if updated.Status != cached.Status && !updated.IsActive() {
				// kick all tenant players
				p.runDisableGameCallbacks(ctx, updated)
			} else if updated.LimitAccess {
				if updated.LimitAccess != cached.LimitAccess || p.isWhitelistUpdated(cached.UsernameWhitelist, updated.UsernameWhitelist) {
					// kick all tenant players
					// TODO: except usernames
					p.runDisableGameCallbacks(ctx, updated)
				}
			}

			err = p.gameMap.Set(ctx, updated.TenantID, updated, p.cacheTTL)
			if err != nil {
				p.logger.For(ctx).Error().
					Err(err).
					Str("tenant_id", updated.TenantID).
					Msg("failed to update game cache")

				err = p.gameMap.Delete(ctx, updated.TenantID)
				if err != nil {
					p.logger.For(ctx).Error().
						Err(err).
						Str("tenant_id", updated.TenantID).
						Msg("failed to clear game cache")
				}

				continue
			}

			p.logger.For(ctx).Debug().
				Str("tenant_id", event.TenantID).
				Msg("game cache updated")
		}
	}
}

func (p *gameProvider) isWhitelistUpdated(cached, updated []string) bool {
	updatedMap := make(map[string]bool, len(updated))
	for _, item := range updated {
		updatedMap[item] = true
	}

	for _, item := range cached {
		if !updatedMap[item] {
			return false
		}
	}

	return true
}
