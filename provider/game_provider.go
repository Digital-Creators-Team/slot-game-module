package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	coreredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	gamemodule "github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/cache"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

// gameProvider implements server.GameProvider using HTTP client
type gameProvider struct {
	gameCode   string
	baseURL    string
	httpClient *http.Client
	cacheTTL   time.Duration
	gameMap    cache.Cache[server.TenantGame]
	logger     logging.LoggerProvider
}

// NewGameProvider creates a new game provider
func NewGameProvider(
	cfg *config.Config,
	module gamemodule.Module,
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
		gameCode: module.GetGameCode(),
		baseURL:  gameConfig.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL: cacheTTL,
		gameMap:  cache.NewTTLMap[server.TenantGame](),
		logger: logging.NewLoggerProvider(logger.With().
			Str("component", "game_provider").
			Str("game_code", module.GetGameCode()).
			Logger(),
		),
	}

	if redisClient != nil && len(gameConfig.EventChannel) > 0 {
		go p.subscribeGameEvent(redisClient, gameConfig.EventChannel)
	}

	return p
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
		return nil, server.ErrTenantNotFound
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
	url := fmt.Sprintf("%s/api/v2/game/%s/%s", p.baseURL, p.gameCode, tenantID)

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

type gameEvent struct {
	GameCode string `json:"game_code"`
	TenantID string `json:"tenant_id"`
	Type     string `json:"type"`
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

			err := p.gameMap.Delete(ctx, event.TenantID)
			if err != nil {
				p.logger.For(ctx).Error().
					Err(err).
					Str("tenant_id", event.TenantID).
					Msg("failed to delete tenant cache")
				continue
			}

			p.logger.For(ctx).Debug().
				Str("tenant_id", event.TenantID).
				Msg("setting cache invalidated")
		}
	}
}
