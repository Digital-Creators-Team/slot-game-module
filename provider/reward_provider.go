package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	coreredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/providers"
	"github.com/Digital-Creators-Team/slot-game-module/server"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// RewardProvider implements server.RewardProvider using HTTP client
type RewardProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
	redis      *coreredis.Client
}

// NewRewardProvider creates a new reward provider
func NewRewardProvider(cfg *config.Config, logger zerolog.Logger, redisClient *coreredis.Client) *RewardProvider {
	timeout := cfg.ExternalServices.RewardService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &RewardProvider{
		baseURL: cfg.ExternalServices.RewardService.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With().Str("component", "reward_provider").Logger(),
		redis:  redisClient,
	}
}

// Contribute adds contribution to jackpot pool
func (p *RewardProvider) Contribute(ctx context.Context, req *providers.ContributeRequest) error {
	// url := fmt.Sprintf("%s/jackpot/contribute", p.baseURL)

	// bodyMap := map[string]interface{}{
	// 	"pool_id":   req.PoolID,
	// 	"amount":    req.Amount.String(),
	// 	"game_code": req.GameCode,
	// 	"user_id":   req.UserID,
	// }

	// // Add spin_id and total_pools if provided
	// if req.SpinID != "" {
	// 	bodyMap["spin_id"] = req.SpinID
	// }
	// if req.TotalPools > 0 {
	// 	bodyMap["total_pools"] = req.TotalPools
	// }

	// body, _ := json.Marshal(bodyMap)

	// httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	// if err != nil {
	// 	return fmt.Errorf("failed to create request: %w", err)
	// }
	// httpReq.Header.Set("Content-Type", "application/json")

	// resp, err := p.httpClient.Do(httpReq)
	// if err != nil {
	// 	return fmt.Errorf("failed to contribute to jackpot: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("contribute failed with status %d", resp.StatusCode)
	// }

	_, err := p.redis.IncrByFloat(ctx, req.PoolID, req.Amount.InexactFloat64())
	if err != nil {
		return fmt.Errorf("failed to contribute to pool in redis: %w", err)
	}

	return nil
}

// Claim claims a jackpot pool and returns the claim
func (p *RewardProvider) Claim(ctx context.Context, req *providers.ClaimRequest) (*server.JackpotClaim, error) {
	// url := fmt.Sprintf("%s/jackpot/claim", p.baseURL)

	// body, _ := json.Marshal(map[string]interface{}{
	// 	"pool_id":    req.PoolID,
	// 	"user_id":    req.UserID,
	// 	"game_code":  req.GameCode,
	// 	"init_value": req.InitValue.String(),
	// })

	// httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create request: %w", err)
	// }
	// httpReq.Header.Set("Content-Type", "application/json")

	// resp, err := p.httpClient.Do(httpReq)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to claim jackpot: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode != http.StatusOK {
	// 	return nil, fmt.Errorf("claim failed with status %d", resp.StatusCode)
	// }

	// var result struct {
	// 	Data server.JackpotClaim `json:"data"`
	// }
	// if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
	// 	return nil, fmt.Errorf("failed to decode response: %w", err)
	// }

	currentVal, err := p.redis.Get(ctx, req.PoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current pool value: %w", err)
	}

	v, _ := decimal.NewFromString(currentVal)

	// if e, _ := p.redis.Exists(ctx, "totalWin"); e {
	// 	_, err := p.redis.IncrByFloat(ctx, "totalWin", v.InexactFloat64())
	// 	if err != nil {
	// 	}
	// } else {
	// 	p.redis.Set(ctx, "totalWin", v.String(), 0)
	// }

	if e, _ := p.redis.Exists(ctx, "jackpotWin"); e {
		_, err := p.redis.IncrByFloat(ctx, "jackpotWin", v.InexactFloat64())
		if err != nil {
		}
	} else {
		p.redis.Set(ctx, "jackpotWin", v.String(), 0)
	}

	d := &server.JackpotClaim{
		PoolID: req.PoolID,
		Amount: v,
	}

	if err := p.redis.Set(ctx, req.PoolID, req.InitValue.String(), 0); err != nil {
		return nil, fmt.Errorf("failed to reset pool in redis: %w", err)
	}

	return d, nil
}

// GetPool retrieves current jackpot pool value
func (p *RewardProvider) GetPool(ctx context.Context, poolID string, initValue decimal.Decimal) (*server.JackpotPool, error) {
	// url := fmt.Sprintf("%s/jackpot/pool/%s?init_value=%s", p.baseURL, poolID, initValue.String())

	// req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create request: %w", err)
	// }

	// resp, err := p.httpClient.Do(req)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get pool: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode != http.StatusOK {
	// 	return nil, fmt.Errorf("reward service returned status %d", resp.StatusCode)
	// }
	// p.redis.HSet(ctx, poolID,  )

	// var result struct {
	// 	Data server.JackpotPool `json:"data"`
	// }
	// if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
	// 	return nil, fmt.Errorf("failed to decode response: %w", err)
	// }
	if err := p.redis.Set(ctx, poolID, initValue.String(), 0); err != nil {
		return nil, fmt.Errorf("failed to cache pool in redis: %w", err)
	}

	d := &server.JackpotPool{
		PoolID: poolID,
		Amount: initValue,
	}

	return d, nil
}
