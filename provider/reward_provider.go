package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/providers"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

// RewardProvider implements server.RewardProvider using HTTP client
type RewardProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     logging.LoggerProvider
}

// NewRewardProvider creates a new reward provider
func NewRewardProvider(cfg *config.Config, logger zerolog.Logger) *RewardProvider {
	timeout := cfg.ExternalServices.RewardService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &RewardProvider{
		baseURL: cfg.ExternalServices.RewardService.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logging.NewLoggerProvider(logger.With().Str("component", "reward_provider").Logger()),
	}
}

// Contribute adds contribution to jackpot pool
func (p *RewardProvider) Contribute(ctx context.Context, req *providers.ContributeRequest) error {
	url := fmt.Sprintf("%s/jackpot/contribute", p.baseURL)

	httpReq, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &req)
	if err != nil {
		return err
	}

	_, err = utils.DoInternalRequest[any](p.logger.For(ctx), p.httpClient, httpReq)
	if err != nil {
		return err
	}

	return nil
}

// Claim claims a jackpot pool and returns the claim
func (p *RewardProvider) Claim(ctx context.Context, req *providers.ClaimRequest) (*server.JackpotClaim, error) {
	url := fmt.Sprintf("%s/jackpot/claim", p.baseURL)

	httpReq, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &req)
	if err != nil {
		return nil, err
	}

	result, err := utils.DoInternalRequest[server.JackpotClaim](p.logger.For(ctx), p.httpClient, httpReq)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// GetPool retrieves current jackpot pool value
func (p *RewardProvider) GetPool(ctx context.Context, poolID string, initValue decimal.Decimal) (*server.JackpotPool, error) {
	url := fmt.Sprintf("%s/jackpot/pool/%s?init_value=%s", p.baseURL, poolID, initValue.String())

	req, err := utils.MakeRequest[any](ctx, p.logger.For(ctx), url, nil)
	if err != nil {
		return nil, err
	}

	result, err := utils.DoInternalRequest[server.JackpotPool](p.logger.For(ctx), p.httpClient, req)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}
