package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/config"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/server"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// RewardProvider implements server.RewardProvider using HTTP client
type RewardProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
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
		logger: logger.With().Str("component", "reward_provider").Logger(),
	}
}

// Contribute adds contribution to jackpot pool
func (p *RewardProvider) Contribute(ctx context.Context, poolID, userID string, amount decimal.Decimal, gameCode string) error {
	url := fmt.Sprintf("%s/jackpot/contribute", p.baseURL)

	body, _ := json.Marshal(map[string]interface{}{
		"pool_id":   poolID,
		"amount":    amount.String(),
		"game_code": gameCode,
		"user_id":   userID,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contribute to jackpot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("contribute failed with status %d", resp.StatusCode)
	}

	return nil
}

// Claim claims a jackpot pool and returns the claim
func (p *RewardProvider) Claim(ctx context.Context, poolID, userID, gameCode string, initValue decimal.Decimal) (*server.JackpotClaim, error) {
	url := fmt.Sprintf("%s/jackpot/claim", p.baseURL)

	body, _ := json.Marshal(map[string]interface{}{
		"pool_id":    poolID,
		"user_id":    userID,
		"game_code":  gameCode,
		"init_value": initValue.String(),
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to claim jackpot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claim failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			ClaimID   string  `json:"claim_id"`
			PoolID    string  `json:"pool_id"`
			UserID    string  `json:"user_id"`
			Amount    float64 `json:"amount"`
			Status    string  `json:"status"`
			CreatedAt string  `json:"created_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, result.Data.CreatedAt)

	return &server.JackpotClaim{
		ClaimID:   result.Data.ClaimID,
		PoolID:    result.Data.PoolID,
		UserID:    result.Data.UserID,
		Amount:    decimal.NewFromFloat(result.Data.Amount),
		Status:    result.Data.Status,
		CreatedAt: createdAt,
	}, nil
}

// GetPool retrieves current jackpot pool value
func (p *RewardProvider) GetPool(ctx context.Context, poolID string, initValue decimal.Decimal) (*server.JackpotPool, error) {
	url := fmt.Sprintf("%s/jackpot/pool/%s?init_value=%s", p.baseURL, poolID, initValue.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reward service returned status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			PoolID    string  `json:"pool_id"`
			Amount    float64 `json:"amount"`
			UpdatedAt string  `json:"updated_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	updatedAt, _ := time.Parse(time.RFC3339, result.Data.UpdatedAt)

	return &server.JackpotPool{
		PoolID:    result.Data.PoolID,
		Amount:    decimal.NewFromFloat(result.Data.Amount),
		UpdatedAt: updatedAt,
	}, nil
}
