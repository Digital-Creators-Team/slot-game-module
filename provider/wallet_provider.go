package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

// WalletProvider implements server.WalletProvider using HTTP client
type WalletProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
}

type ErrorResponse struct {
	StatusCode int         `json:"status_code"`
	IsSuccess  bool        `json:"is_success"`
	Error      ErrorDetail `json:"error,omitempty"`
}

type ErrorDetail struct {
	Timestamp    string `json:"timestamp"`
	Path         string `json:"path"`
	ErrorMessage string `json:"error_message"`
}

// NewWalletProvider creates a new wallet provider
func NewWalletProvider(cfg *config.Config, logger zerolog.Logger) *WalletProvider {
	timeout := cfg.ExternalServices.WalletService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &WalletProvider{
		baseURL: cfg.ExternalServices.WalletService.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger.With().Str("component", "wallet_provider").Logger(),
	}
}

// GetBalance retrieves player balance from wallet service
func (p *WalletProvider) GetBalance(ctx context.Context, userID, currencyID string) (decimal.Decimal, error) {
	url := fmt.Sprintf("%s/wallet/balance?user_id=%s&currency_id=%s", p.baseURL, userID, currencyID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
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

// CheckBalance retrieves player balance from wallet service
func (p *WalletProvider) CheckBalance(ctx context.Context, productId, username, currencyID string) (decimal.Decimal, error) {
	url := fmt.Sprintf("%s/wallet/checkBalance", p.baseURL)

	id := uuid.NewString()
	reqBody := map[string]any{
		"id":              id,
		"timestampMillis": time.Now().UnixNano() / 1000000,
		"productId":       productId,
		"currency":        currencyID,
		"username":        username,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return decimal.Zero, fmt.Errorf("wallet service returned status %d", resp.StatusCode)
	}

	var result struct {
		Balance float64 `json:"balance"` // External service returns float64
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return decimal.Zero, fmt.Errorf("failed to decode response: %w", err)
	}

	return decimal.NewFromFloat(result.Balance), nil
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/withdraw", p.baseURL)

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
		return fmt.Errorf("failed to withdraw: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("withdraw failed with status %d", resp.StatusCode)
	}
	switch strings.ToLower(errResp.Error.ErrorMessage) {
	case ErrInsufficientFunds.Error():
		return ErrInsufficientFunds
	default:
		return fmt.Errorf("withdraw failed: %s", errResp.Error.ErrorMessage)
	}
}

// Deposit adds amount to player balance
func (p *WalletProvider) Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/deposit", p.baseURL)

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
		return fmt.Errorf("failed to deposit: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deposit failed with status %d", resp.StatusCode)
	}

	return nil
}
