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
	"github.com/Digital-Creators-Team/slot-game-module/pkg/cache"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

// tenantProvider implements server.TenantProvider using HTTP client
type tenantProvider struct {
	baseURL      string
	whitelistMap map[string]bool
	blacklistMap map[string]bool
	httpClient   *http.Client
	cacheTTL     time.Duration
	tenantMap    cache.Cache[server.ResponseTenant]
	logger       zerolog.Logger
}

// NewTenantProvider creates a new tenant provider
func NewTenantProvider(
	cfg *config.Config,
	logger zerolog.Logger,
	redisClient *coreredis.Client,
) server.TenantProvider {
	tenantConfig := cfg.ExternalServices.TenantService
	timeout := cfg.ExternalServices.WalletService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	cacheTTL := tenantConfig.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = 5 * time.Minute
	}

	p := &tenantProvider{
		baseURL: tenantConfig.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cacheTTL:  cacheTTL,
		tenantMap: cache.NewTTLMap[server.ResponseTenant](),
		logger:    logger.With().Str("component", "tenant_provider").Logger(),
	}

	p.whitelistMap = make(map[string]bool, len(tenantConfig.Whitelist))
	for _, w := range tenantConfig.Whitelist {
		p.whitelistMap[w] = true
	}

	p.blacklistMap = make(map[string]bool, len(tenantConfig.Blacklist))
	for _, b := range tenantConfig.Blacklist {
		p.blacklistMap[b] = true
	}

	if redisClient != nil && len(tenantConfig.EventChannel) > 0 {
		go p.subscribeTenantEvent(redisClient, tenantConfig.EventChannel)
	}

	return p
}

func (p *tenantProvider) Get(ctx context.Context, id string, skipCache bool) (*server.ResponseTenant, error) {
	if !p.isAllowed(id) {
		return nil, server.ErrTenantNotFound
	}

	if !skipCache {
		cached, err := p.tenantMap.Get(ctx, id)
		if err == nil {
			return &cached, nil
		}
		p.logger.Warn().
			Err(err).
			Str("tenant_id", id).
			Msg("tenant cache miss")
	}

	tenant, err := p.get(ctx, id)
	if err != nil {
		return nil, err
	}

	if tenant == nil {
		return nil, server.ErrTenantNotFound
	}

	err = p.tenantMap.Set(ctx, id, *tenant, p.cacheTTL)
	if err != nil {
		p.logger.Warn().
			Err(err).
			Str("tenant_id", id).
			Msg("failed to set tenant cache")
	}
	return tenant, nil
}

func (p *tenantProvider) get(ctx context.Context, id string) (*server.ResponseTenant, error) {
	url := fmt.Sprintf("%s/api/v1/tenant/%s", p.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		p.logger.Error().Err(err).Msg("failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	respData, err := utils.DoInternalRequest[server.ResponseTenant](p.logger, p.httpClient, req)
	if err != nil {
		return nil, err
	}

	return &respData.Data, nil
}

func (p *tenantProvider) isAllowed(tenantID string) bool {
	if p.blacklistMap[tenantID] {
		return false
	}

	if len(p.whitelistMap) > 0 && !p.whitelistMap[tenantID] {
		return false
	}

	return true
}

type tenantEvent struct {
	TenantID string `json:"tenant_id"`
	Event    string `json:"event"`
}

func (p *tenantProvider) subscribeTenantEvent(redisClient *coreredis.Client, eventChannel string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps := redisClient.GetClient().Subscribe(ctx, eventChannel)
	defer func(ps *redis.PubSub) {
		err := ps.Close()
		if err != nil {
			p.logger.Error().Err(err).Msg("failed to close tenant event redis subscription")
		}
	}(ps)

	ch := ps.Channel()

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-ch:
			var event tenantEvent

			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				p.logger.Error().
					Err(err).
					Msg("failed to parse tenant refresh event")

				continue
			}

			switch event.Event {
			case "update":
				break
			default:
				continue
			}

			err := p.tenantMap.Delete(ctx, event.TenantID)
			if err != nil {
				p.logger.Error().
					Err(err).
					Str("tenant_id", event.TenantID).
					Msg("failed to delete tenant cache")
				continue
			}

			p.logger.Debug().
				Str("tenant_id", event.TenantID).
				Msg("setting cache invalidated")
		}
	}
}
