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

	zeussdk "github.com/Digital-Creators-Team/zeus-integration-sdk/client"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	moduleerrors "github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

var ErrInsufficientFunds = errors.New("insufficient funds")

// WalletProvider implements server.WalletProvider using HTTP client
type WalletProvider struct {
	tenant     *server.ResponseTenant
	baseURL    string
	httpClient *http.Client
	logger     logging.LoggerProvider
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
		logger: logging.NewLoggerProvider(logger.With().Str("component", "wallet_provider").Logger()),
	}
}

// CheckBalance retrieves player balance from wallet service
func (p *WalletProvider) CheckBalance(
	ctx context.Context,
	gameCode string,
	userID string,
	username string,
	currencyID string,
	tenantToken string,
) (decimal.Decimal, error) {
	if p.tenant == nil {
		return p.CheckBalanceV2(ctx, p.baseURL, gameCode, username, currencyID)
	}

	switch p.tenant.WalletVersion {
	case 2:
		return p.CheckBalanceV2(ctx, p.tenant.WalletCallbackURL, gameCode, username, currencyID)
	case 3:
		return p.CheckBalanceV3(ctx, tenantToken)
	default:
		return p.GetBalanceV1(ctx, userID, currencyID)
	}
}

// GetBalanceV1 retrieves player balance from wallet service
func (p *WalletProvider) GetBalanceV1(ctx context.Context, userID, currencyID string) (decimal.Decimal, error) {
	url := fmt.Sprintf("%s/wallet/balance?user_id=%s&currency_id=%s", p.baseURL, userID, currencyID)

	req, err := utils.MakeRequest[any](ctx, p.logger.For(ctx), url, nil)
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

func (p *WalletProvider) CheckBalanceV2(
	ctx context.Context,
	baseURL string,
	productID string,
	username string,
	currencyID string,
) (decimal.Decimal, error) {
	url := fmt.Sprintf("%s/checkBalance", baseURL)

	id := uuid.NewString()
	requestBody := map[string]any{
		"id":              id,
		"timestampMillis": time.Now().UnixNano() / 1000000,
		"productId":       productID,
		"currency":        currencyID,
		"username":        username,
	}

	p.logger.For(ctx).Debug().Str("url", url).Any("request", requestBody).Msg("check balance request")

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &requestBody)
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

	p.logger.For(ctx).Debug().Bytes("response_bytes", responseBytes).Msg("check balance response")

	err = json.Unmarshal(responseBytes, &result)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	if result.StatusCode == (int)(moduleerrors.InsufficientBalance) || (result.StatusCode == (int)(moduleerrors.InternalServerError) && result.Balance == 0) {
		return decimal.Zero, ErrInsufficientFunds
	} else if result.StatusCode != (int)(moduleerrors.Success) {
		return decimal.Zero, fmt.Errorf("wallet service returned status %d", result.StatusCode)
	}

	p.logger.For(ctx).Debug().Any("result", result).Msg("check balance result")

	return decimal.NewFromFloat(result.Balance), nil
}

func (p *WalletProvider) CheckBalanceV3(
	ctx context.Context,
	tenantToken string,
) (decimal.Decimal, error) {
	respV3, err := p.newZeusClient(ctx).CheckBalance(ctx, tenantToken)
	if err != nil {
		return decimal.Zero, err
	}

	return respV3.AvailableBalance, nil
}

// PlaceBets place bets
func (p *WalletProvider) PlaceBets(
	ctx context.Context,
	gameCode string,
	userID string,
	username string,
	currencyID string,
	amount decimal.Decimal,
	sessionID string,
	roundID string,
	betID string,
	tenantToken string,
) (string, decimal.Decimal, error) {
	if p.tenant == nil {
		return p.PlaceBetsV2(ctx, p.baseURL, gameCode, username, currencyID, amount, roundID, betID)
	}

	switch p.tenant.WalletVersion {
	case 3:
		return p.PlaceBetsV3(ctx, betID, roundID, sessionID, amount, tenantToken)
	case 2:
		fallthrough
	default:
		return p.PlaceBetsV2(ctx, p.tenant.WalletCallbackURL, gameCode, username, currencyID, amount, roundID, betID)
	}
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) PlaceBetsV2(
	ctx context.Context,
	baseURL string,
	productID string,
	userName string,
	currencyID string,
	amount decimal.Decimal,
	roundID string,
	transactionID string,
) (string, decimal.Decimal, error) {
	url := fmt.Sprintf("%s/placeBets", baseURL)

	requestBody := map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productID,
		"username":        userName,
		"currency":        currencyID,
		"txns": []map[string]interface{}{
			{
				"id":          transactionID,
				"gameCode":    productID,
				"status":      "OPEN",
				"roundId":     roundID,
				"betAmount":   amount.InexactFloat64(), // docs is int, now using float
				"playInfo":    productID,
				"isFreespins": false,
			},
		},
	}

	p.logger.For(ctx).Debug().Str("url", url).Any("request", requestBody).Msg("place bets request")

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &requestBody)
	if err != nil {
		return "", decimal.Zero, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", decimal.Zero, fmt.Errorf("failed to withdraw: %w", err)
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
		return "", decimal.Zero, fmt.Errorf("failed to decode response body: %w", err)
	}

	p.logger.For(ctx).Debug().Any("result", result).Msg("place bets result")

	if resp.StatusCode == http.StatusOK && result.StatusCode == (int)(moduleerrors.Success) {
		return result.ID, decimal.NewFromFloat(result.BalanceAfter), nil
	}

	if result.StatusCode == (int)(moduleerrors.InsufficientBalance) || result.StatusCode == (int)(moduleerrors.InternalServerError) {
		return "", decimal.Zero, ErrInsufficientFunds
	} else if result.StatusCode != (int)(moduleerrors.Success) {
		return "", decimal.Zero, fmt.Errorf("wallet service returned status %d", result.StatusCode)
	}

	return "", decimal.Zero, fmt.Errorf("withdraw failed: %d", result.StatusCode)
}

func (p *WalletProvider) PlaceBetsV3(
	ctx context.Context,
	betID string,
	roundID string,
	sessionID string,
	amount decimal.Decimal,
	tenantToken string,
) (string, decimal.Decimal, error) {
	zeusClient := p.newZeusClient(ctx)

	respV3, err := zeusClient.PlaceBets(
		ctx,
		tenantToken,
		zeusClient.NewRequestID(),
		betID,
		roundID,
		sessionID,
		amount,
		nil,
	)
	if err != nil {
		return "", decimal.Zero, err
	}

	return respV3.ReferenceBetID, respV3.BalanceAfter, nil
}

func (p *WalletProvider) SettleBets(
	ctx context.Context,
	gameCode string,
	userID string,
	username string,
	currencyID string,
	betAmount decimal.Decimal,
	payoutAmount decimal.Decimal,
	sessionID string,
	roundID string,
	betID string,
	referenceBetID string,
	tenantToken string,
) (string, decimal.Decimal, error) {
	if p.tenant == nil {
		return p.SettleBetsV2(ctx, p.baseURL, gameCode, username, currencyID, betAmount, payoutAmount, roundID, betID)
	}

	switch p.tenant.WalletVersion {
	case 3:
		return p.SettleBetsV3(ctx, payoutAmount, betID, referenceBetID, tenantToken)
	case 2:
		fallthrough
	default:
		return p.SettleBetsV2(ctx, p.tenant.WalletCallbackURL, gameCode, username, currencyID, betAmount, payoutAmount, roundID, betID)
	}
}

func (p *WalletProvider) SettleBetsV2(
	ctx context.Context,
	baseURL string,
	productID string,
	username string,
	currencyID string,
	betAmount decimal.Decimal,
	payoutAmount decimal.Decimal,
	roundID string,
	transactionID string,
) (string, decimal.Decimal, error) {
	url := fmt.Sprintf("%s/settleBets", baseURL)

	requestBody := map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productID,
		"username":        username,
		"currency":        currencyID,
		"txns": []map[string]interface{}{
			{
				"id":              transactionID,
				"gameCode":        productID,
				"status":          "SETTLED",
				"roundId":         roundID,
				"betAmount":       betAmount.InexactFloat64(), // docs is int, now using float
				"payoutAmount":    payoutAmount.InexactFloat64(),
				"winlost":         payoutAmount.InexactFloat64() - betAmount.InexactFloat64(),
				"playInfo":        productID,
				"turnOver":        betAmount.InexactFloat64(),
				"isSingleState":   false,
				"transactionType": "BY_TRANSACTION",
				"isFreespins":     betAmount.Equal(decimal.Zero),
			},
		},
	}

	p.logger.For(ctx).Debug().Str("url", url).Any("request", requestBody).Msg("settle bets request")

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &requestBody)
	if err != nil {
		return "", decimal.Zero, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", decimal.Zero, fmt.Errorf("failed to deposit: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		ID            string          `json:"id"`
		StatusCode    int             `json:"statusCode"`
		ProductID     string          `json:"productId"`
		BalanceBefore decimal.Decimal `json:"balanceBefore"`
		BalanceAfter  decimal.Decimal `json:"balanceAfter"`
	}

	//var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", decimal.Zero, fmt.Errorf("failed to decode response body: %w", err)
	}

	p.logger.For(ctx).Debug().Any("result", result).Msg("settle bets result")

	if resp.StatusCode != http.StatusOK {
		return "", decimal.Zero, fmt.Errorf("settle bets response status %d", resp.StatusCode)
	}

	if result.StatusCode != (int)(moduleerrors.Success) {
		return "", decimal.Zero, fmt.Errorf("wallet service returned status %d", result.StatusCode)
	}

	return "", result.BalanceAfter, nil
}

func (p *WalletProvider) SettleBetsV3(
	ctx context.Context,
	payoutAmount decimal.Decimal,
	betID string,
	referenceBetID string,
	tenantToken string,
) (string, decimal.Decimal, error) {
	zeusClient := p.newZeusClient(ctx)

	resp, err := zeusClient.SettleBets(
		ctx,
		tenantToken,
		zeusClient.NewRequestID(),
		betID,
		referenceBetID,
		payoutAmount,
		nil,
	)
	if err != nil {
		return "", decimal.Zero, err
	}

	return resp.ReferenceBetID, resp.BalanceAfter, nil
}

func (p *WalletProvider) CancelBets(
	ctx context.Context,
	gameCode string,
	userID string,
	username string,
	currencyID string,
	betAmount decimal.Decimal,
	payoutAmount decimal.Decimal,
	sessionID string,
	roundID string,
	betID string,
	referenceBetID string,
	tenantToken string,
) (string, decimal.Decimal, error) {
	if p.tenant == nil {
		return p.SettleBetsV2(ctx, p.baseURL, gameCode, username, currencyID, betAmount, payoutAmount, roundID, betID)
	}

	switch p.tenant.WalletVersion {
	case 3:
		return p.CancelBetsV3(ctx, betID, referenceBetID, tenantToken)
	case 2:
		fallthrough
	default:
		return p.SettleBetsV2(ctx, p.tenant.WalletCallbackURL, gameCode, username, currencyID, betAmount, payoutAmount, roundID, betID)
	}
}

func (p *WalletProvider) CancelBetsV3(
	ctx context.Context,
	betID string,
	referenceBetID string,
	tenantToken string,
) (string, decimal.Decimal, error) {
	zeusClient := p.newZeusClient(ctx)

	resp, err := zeusClient.CancelBets(
		ctx,
		tenantToken,
		zeusClient.NewRequestID(),
		betID,
		referenceBetID,
	)
	if err != nil {
		return "", decimal.Zero, err
	}

	return resp.ReferenceBetID, resp.BalanceAfter, nil
}

// Withdraw deducts amount from player balance
func (p *WalletProvider) Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/withdraw", p.baseURL)

	requestBody := map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	}

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &requestBody)
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

// Deposit adds amount to player balance
func (p *WalletProvider) Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error {
	url := fmt.Sprintf("%s/wallet/deposit", p.baseURL)

	requestBody := map[string]interface{}{
		"user_id":     userID,
		"currency_id": currencyID,
		"amount":      amount.InexactFloat64(), // Convert to float64 for external service
	}

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &requestBody)
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

func (p *WalletProvider) WithTenant(ctx context.Context, provider server.TenantProvider, tenantID string) (server.WalletProvider, error) {
	if provider == nil {
		p.logger.For(ctx).Warn().
			Str("tenant_id", tenantID).
			Msg("Tenant provider is nil")
		return p, nil
	}

	tenant, err := provider.Get(ctx, tenantID, false)
	if err != nil {
		p.logger.For(ctx).Error().
			Err(err).
			Str("tenant_id", tenantID).
			Msg("Failed to get tenant info")
		return nil, err
	}

	if !tenant.WalletEnabled() {
		p.logger.For(ctx).Error().
			Str("tenant_id", tenantID).
			Str("status", tenant.Status).
			Msg("Tenant wallet not enabled")
		return nil, server.ErrTenantWalletNotEnabled
	}

	return &WalletProvider{
		tenant:     tenant,
		baseURL:    tenant.WalletCallbackURL,
		httpClient: p.httpClient,
		logger: p.logger.With(func(loggerContext zerolog.Context) zerolog.Context {
			return loggerContext.Str("tenant_id", tenantID)
		}),
	}, nil
}

func (p *WalletProvider) newZeusClient(ctx context.Context) zeussdk.Client {
	logger := p.logger.For(ctx)
	return zeussdk.NewWithHTTPClient(
		&zeussdk.Config{
			ClientID:     p.tenant.SecretMetadata[server.TenantMetadataWalletClientID],
			ClientAPIKey: p.tenant.SecretMetadata[server.TenantMetadataWalletClientAPIKey],
			BaseURL:      p.baseURL,
		},
		*logger,
		p.httpClient,
	)
}
