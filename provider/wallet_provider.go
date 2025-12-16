package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/config"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// WalletProvider implements server.WalletProvider using HTTP client
type WalletProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
}

// NewWalletProvider creates a new wallet provider
func NewWalletProvider(cfg *config.Config, logger zerolog.Logger) *WalletProvider {
	timeout := cfg.ExternalServices.WalletService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	baseURL := cfg.ExternalServices.WalletService.BaseURL
	if baseURL == "" {
		logger.Warn().Msg("Wallet service base URL is empty - wallet operations will fail")
	} else {
		logger.Info().Str("base_url", baseURL).Msg("Wallet provider initialized")
	}

	return &WalletProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With().Str("component", "wallet_provider").Logger(),
	}
}

// GetBalance retrieves player balance from wallet service
func (p *WalletProvider) GetBalance(ctx context.Context, userID, currencyID string) (decimal.Decimal, error) {
	if p.baseURL == "" {
		return decimal.Zero, fmt.Errorf("wallet service base URL is not configured")
	}

	url := fmt.Sprintf("%s/api/wallet/balance?user_id=%s&currency_id=%s", p.baseURL, userID, currencyID)
	p.logger.Debug().Str("url", url).Msg("Getting balance from wallet service")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error().Err(err).Str("url", url).Msg("Failed to call wallet service")
		return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("url", url).
			Msg("Wallet service returned non-OK status")
		return decimal.Zero, fmt.Errorf("wallet service returned status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Balance float64 `json:"balance"` // External service returns float64
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return decimal.Zero, fmt.Errorf("failed to decode response: %w", err)
	}

	return decimal.NewFromFloat(result.Data.Balance), nil
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	if p.baseURL == "" {
		return fmt.Errorf("wallet service base URL is not configured")
	}

	url := fmt.Sprintf("%s/api/wallet/withdraw", p.baseURL)
	p.logger.Debug().Str("url", url).Str("user_id", userID).Str("amount", amount.String()).Msg("Withdrawing from wallet")

	body, _ := json.Marshal(map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error().Err(err).Str("url", url).Msg("Failed to call wallet service for withdraw")
		return fmt.Errorf("failed to withdraw: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("url", url).
			Msg("Wallet service returned non-OK status for withdraw")
		return fmt.Errorf("withdraw failed with status %d", resp.StatusCode)
	}

	return nil
}

// Deposit adds amount to player balance
func (p *WalletProvider) Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	if p.baseURL == "" {
		return fmt.Errorf("wallet service base URL is not configured")
	}

	url := fmt.Sprintf("%s/api/wallet/deposit", p.baseURL)
	p.logger.Debug().Str("url", url).Str("user_id", userID).Str("amount", amount.String()).Msg("Depositing to wallet")

	body, _ := json.Marshal(map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error().Err(err).Str("url", url).Msg("Failed to call wallet service for deposit")
		return fmt.Errorf("failed to deposit: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("url", url).
			Msg("Wallet service returned non-OK status for deposit")
		return fmt.Errorf("deposit failed with status %d", resp.StatusCode)
	}

	return nil
}
