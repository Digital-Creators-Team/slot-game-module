package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	coreredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

// WalletProvider implements server.WalletProvider using HTTP client
type WalletProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     zerolog.Logger
	redis      *coreredis.Client
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
func NewWalletProvider(cfg *config.Config, logger zerolog.Logger, redisClient *coreredis.Client) *WalletProvider {
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
		redis:  redisClient,
	}
}

// GetBalance retrieves player balance from wallet service
func (p *WalletProvider) GetBalance(ctx context.Context, userID, currencyID string) (decimal.Decimal, error) {
	// url := fmt.Sprintf("%s/wallet/balance?user_id=%s&currency_id=%s", p.baseURL, userID, currencyID)

	// req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	// if err != nil {
	// 	return decimal.Zero, fmt.Errorf("failed to create request: %w", err)
	// }

	// resp, err := p.httpClient.Do(req)
	// if err != nil {
	// 	return decimal.Zero, fmt.Errorf("failed to get balance: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode != http.StatusOK {
	// 	return decimal.Zero, fmt.Errorf("wallet service returned status %d", resp.StatusCode)
	// }

	// var result struct {
	// 	Data struct {
	// 		Balance float64 `json:"balance"` // External service returns float64
	// 	} `json:"data"`
	// }
	// if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
	// 	return decimal.Zero, fmt.Errorf("failed to decode response: %w", err)
	// }

	currentVal, err := p.redis.Get(ctx, userID)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get current pool value: %w", err)
	}

	v, _ := decimal.NewFromString(currentVal)

	return v, nil
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	// url := fmt.Sprintf("%s/wallet/withdraw", p.baseURL)

	// body, _ := json.Marshal(map[string]interface{}{
	// 	"user_id":     userID,
	// 	"currency_id": currencyID,
	// 	"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	// })

	// req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	// if err != nil {
	// 	return fmt.Errorf("failed to create request: %w", err)
	// }
	// req.Header.Set("Content-Type", "application/json")

	// resp, err := p.httpClient.Do(req)
	// if err != nil {
	// 	return fmt.Errorf("failed to withdraw: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode == http.StatusOK {
	// 	return nil
	// }
	// var errResp ErrorResponse
	// if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
	// 	return fmt.Errorf("withdraw failed with status %d", resp.StatusCode)
	// }
	// switch strings.ToLower(errResp.Error.ErrorMessage) {
	// case ErrInsufficientFunds.Error():
	// 	return ErrInsufficientFunds
	// default:
	// 	return fmt.Errorf("withdraw failed: %s", errResp.Error.ErrorMessage)
	// }

	// _, err := p.redis.IncrByFloat(ctx, userID, amount.Neg().InexactFloat64())
	// if err != nil {
	// 	return fmt.Errorf("failed to contribute to pool in redis: %w", err)
	// }

	// if e, _ := p.redis.Exists(ctx, "totalSpend"); e {
	// 	_, err := p.redis.IncrByFloat(ctx, "totalSpend", amount.InexactFloat64())
	// 	if err != nil {
	// 	}
	// } else {
	// 	p.redis.Set(ctx, "totalSpend", amount.String(), 0)
	// }

	return nil
}

// Deposit adds amount to player balance
func (p *WalletProvider) Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	// url := fmt.Sprintf("%s/wallet/deposit", p.baseURL)

	// body, _ := json.Marshal(map[string]interface{}{
	// 	"user_id":     userID,
	// 	"currency_id": currencyID,
	// 	"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	// })

	// req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	// if err != nil {
	// 	return fmt.Errorf("failed to create request: %w", err)
	// }
	// req.Header.Set("Content-Type", "application/json")

	// resp, err := p.httpClient.Do(req)
	// if err != nil {
	// 	return fmt.Errorf("failed to deposit: %w", err)
	// }
	// defer func() { _ = resp.Body.Close() }()

	// if resp.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("deposit failed with status %d", resp.StatusCode)
	// }

	// _, err := p.redis.IncrByFloat(ctx, userID, amount.InexactFloat64())
	// if err != nil {
	// 	return fmt.Errorf("failed to contribute to pool in redis: %w", err)
	// }

	// if e, _ := p.redis.Exists(ctx, "totalWin"); e {
	// 	_, err := p.redis.IncrByFloat(ctx, "totalWin", amount.InexactFloat64())
	// 	if err != nil {
	// 	}
	// } else {
	// 	p.redis.Set(ctx, "totalWin", amount.String(), 0)
	// }

	return nil
}
