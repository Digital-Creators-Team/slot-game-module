package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	moduleerrors "github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
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

	req, err := utils.MakeRequest[any](ctx, p.logger, url, nil)
	if err != nil {
		return decimal.Zero, err
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
func (p *WalletProvider) CheckBalance(ctx context.Context, productId, tenantID, username, currencyID string) (decimal.Decimal, error) {
	url := fmt.Sprintf("%s/checkBalance", p.baseURL)

	id := uuid.NewString()
	requestBody := map[string]any{
		"id":              id,
		"timestampMillis": time.Now().UnixNano() / 1000000,
		"productId":       productId,
		"currency":        currencyID,
		"tenantId":        tenantID,
		"username":        username,
	}

	p.logger.Debug().Str("url", url).Any("request", requestBody).Msg("check balance request")

	req, err := utils.MakeRequest(ctx, p.logger, url, &requestBody)
	if err != nil {
		return decimal.Zero, err
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
		StatusCode int     `json:"statusCode"`
		Balance    float64 `json:"balance"` // External service returns float64
	}

	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to read response body: %w", err)
	}

	p.logger.Debug().Bytes("response_bytes", responseBytes).Msg("check balance response")

	err = json.Unmarshal(responseBytes, &result)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	if result.StatusCode == (int)(moduleerrors.InsufficientBalance) || (result.StatusCode == (int)(moduleerrors.InternalServerError) && result.Balance == 0) {
		return decimal.Zero, ErrInsufficientFunds
	} else if result.StatusCode != (int)(moduleerrors.Success) {
		return decimal.Zero, fmt.Errorf("wallet service returned status %d", result.StatusCode)
	}

	p.logger.Debug().Any("result", result).Msg("check balance result")

	return decimal.NewFromFloat(result.Balance), nil
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/withdraw", p.baseURL)

	requestBody := map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	}

	req, err := utils.MakeRequest(ctx, p.logger, url, &requestBody)
	if err != nil {
		return err
	}

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

// Withdraw deducts amount from player balance
func (p *WalletProvider) PlaceBets(ctx context.Context, productId, tenantID, userName, currencyID string, amount decimal.Decimal, roundID string, transactionId string, gameCode string, gameName string) error {
	url := fmt.Sprintf("%s/placeBets", p.baseURL)

	requestBody := map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productId,
		"tenantId":        tenantID,
		"username":        userName,
		"currency":        currencyID,
		"txns": []map[string]interface{}{
			{
				"id":          transactionId,
				"gameCode":    gameCode,
				"status":      "OPEN",
				"roundId":     roundID,
				"betAmount":   amount.InexactFloat64(), // docs is int, now using float
				"playInfo":    gameName,
				"isFreespins": false,
			},
		},
	}

	p.logger.Debug().Str("url", url).Any("request", requestBody).Msg("place bets request")

	req, err := utils.MakeRequest(ctx, p.logger, url, &requestBody)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to withdraw: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	/*if resp.StatusCode == http.StatusOK {
		return nil
	}*/

	var result struct {
		ID            string  `json:"id"`
		StatusCode    int     `json:"statusCode"`
		BalanceBefore float64 `json:"balanceBefore"` // External service returns float64
		BalanceAfter  float64 `json:"balanceAfter"`
	}
	//var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	p.logger.Debug().Any("result", result).Msg("place bets result")

	if resp.StatusCode == http.StatusOK && result.StatusCode == (int)(moduleerrors.Success) {
		return nil
	}

	if result.StatusCode == (int)(moduleerrors.InsufficientBalance) || result.StatusCode == (int)(moduleerrors.InternalServerError) {
		return ErrInsufficientFunds
	} else if result.StatusCode != (int)(moduleerrors.Success) {
		return fmt.Errorf("wallet service returned status %d", result.StatusCode)
	}

	return fmt.Errorf("withdraw failed: %d", result.StatusCode)
}

// Deposit adds amount to player balance
func (p *WalletProvider) Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/deposit", p.baseURL)

	requestBody := map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	}

	req, err := utils.MakeRequest(ctx, p.logger, url, &requestBody)
	if err != nil {
		return err
	}

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

func (p *WalletProvider) SettleBets(ctx context.Context, productId, tenantID, username, currencyID string, amount decimal.Decimal, payoutAmount decimal.Decimal, roundID string, transactionId string, gameCode string, gameName string) error {
	url := fmt.Sprintf("%s/settleBets", p.baseURL)

	requestBody := map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productId,
		"tenantId":        tenantID,
		"username":        username,
		"currency":        currencyID,
		"txns": []map[string]interface{}{
			{
				"id":              transactionId,
				"gameCode":        gameCode,
				"status":          "SETTLED",
				"roundId":         roundID,
				"betAmount":       amount.InexactFloat64(), // docs is int, now using float
				"payoutAmount":    payoutAmount.InexactFloat64(),
				"winlost":         payoutAmount.InexactFloat64() - amount.InexactFloat64(),
				"playInfo":        gameName,
				"turnOver":        amount.InexactFloat64(),
				"isSingleState":   false,
				"transactionType": "BY_TRANSACTION",
				"isFreespins":     false,
			},
		},
	}

	p.logger.Debug().Str("url", url).Any("request", requestBody).Msg("settle bets request")

	req, err := utils.MakeRequest(ctx, p.logger, url, &requestBody)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to deposit: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		ID            string  `json:"id"`
		StatusCode    int     `json:"statusCode"`
		ProductId     string  `json:"productId"`
		BalanceBefore float64 `json:"balanceBefore"` // External service returns float64
		BalanceAfter  float64 `json:"balanceAfter"`
	}

	//var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response body: %w", err)
	}

	p.logger.Debug().Any("result", result).Msg("settle bets result")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deposit failed with status %d", resp.StatusCode)
	}

	return nil
}

func (p *WalletProvider) GetWalletUrl(ctx context.Context) string {
	return p.baseURL
}

func (p *WalletProvider) WithTenant(ctx context.Context, provider server.TenantProvider, tenantID string) (server.WalletProvider, error) {
	if provider == nil {
		p.logger.Warn().
			Str("tenant_id", tenantID).
			Msg("Tenant provider is nil")
		return p, nil
	}

	tenant, err := provider.Get(ctx, tenantID, false)
	if err != nil {
		p.logger.Error().
			Err(err).
			Str("tenant_id", tenantID).
			Msg("Failed to get tenant info")
		return nil, err
	}

	if !tenant.WalletEnabled() {
		p.logger.Error().
			Str("tenant_id", tenantID).
			Str("status", tenant.Status).
			Msg("Tenant wallet not enabled")
		return nil, server.ErrTenantWalletNotEnabled
	}

	return &WalletProvider{
		baseURL:    tenant.WalletCallbackURL,
		httpClient: p.httpClient,
		logger:     p.logger.With().Str("tenant_id", tenantID).Logger(),
	}, nil
}
