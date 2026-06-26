package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	//url := fmt.Sprintf("%s/wallet/checkBalance", p.baseURL)
	url := fmt.Sprintf("%s/sexy/checkBalance", p.baseURL) //TODO, replace sexy by productID or other
	fmt.Printf("===> CheckBalance, data check: %s %s\n", url, productId)

	id := uuid.NewString()
	reqBody := map[string]any{
		"id":              id,
		"timestampMillis": time.Now().UnixNano() / 1000000,
		"productId":       productId,
		"currency":        currencyID,
		"username":        username,
	}

	fmt.Printf("===> CheckBalance, data check v1.0: %+v \n", reqBody)

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
	fmt.Printf("===> CheckBalance, data check v1.1: %+v \n", resp.Body)
	//
	bodyBytes2, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("read body error:", err)
		return decimal.Zero, fmt.Errorf("wallet service returned status 22 %d", resp.StatusCode)
	}

	fmt.Println("response body:", string(bodyBytes2))

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes2))
	//
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return decimal.Zero, fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("===> CheckBalance, data check v2: %+v \n ===> %+v\n", result, resp.Body)

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

// Withdraw deducts amount from player balance
func (p *WalletProvider) PlaceBets(ctx context.Context, productId, userName, currencyID string, amount decimal.Decimal, roundID string) error {
	url := fmt.Sprintf("%s/sexy/placeBets", p.baseURL)
	fmt.Printf("===> PlaceBets, data check: %s\n", url)

	body, _ := json.Marshal(map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productId,
		"username":        userName,
		"currency":        currencyID,
		//"amount":          amount.InexactFloat64(), // Convert to float64 for external service
		// edit info later
		"txns": []map[string]interface{}{
			{
				"id":        "T-001",
				"gameCode":  "MX-LIVE-001",
				"status":    "OPEN",
				"roundId":   roundID,
				"betAmount": amount.InexactFloat64(), // docs is int, now using float
				"playInfo":  "Golden Coyote",
			},
		},
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

	fmt.Printf("===> PlaceBets, data check v2: %+v\n", resp)

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

func (p *WalletProvider) SettleBets(ctx context.Context, productId, username, currencyID string, amount decimal.Decimal, payoutAmount decimal.Decimal, roundID string) error {
	url := fmt.Sprintf("%s/sexy/settleBets", p.baseURL)

	body, _ := json.Marshal(map[string]interface{}{
		"id":              uuid.New().String(),
		"timestampMillis": time.Now().UnixMilli(),
		"productId":       productId,
		"username":        username,
		"currency":        currencyID,
		//"amount":          amount.InexactFloat64(), // Convert to float64 for external service
		// edit info later
		"txns": []map[string]interface{}{
			{
				"id":              "T-001",
				"gameCode":        "MX-LIVE-001",
				"status":          "SETTLED",
				"roundId":         roundID,
				"betAmount":       amount.InexactFloat64(), // docs is int, now using float
				"payoutAmount":    payoutAmount.InexactFloat64(),
				"playInfo":        "Golden Coyote",
				"turnOver":        amount.InexactFloat64(),
				"isSingleState":   false,
				"transactionType": "BY_TRANSACTION",
			},
		},
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
