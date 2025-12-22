package providers

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// StateProvider interface for player state management
type StateProvider interface {
	GetPlayerState(ctx context.Context, userID, gameCode string) (interface{}, error)
	SavePlayerState(ctx context.Context, userID, gameCode string, state interface{}) error
}

// WalletProvider interface for wallet operations
type WalletProvider interface {
	GetBalance(ctx context.Context, userID, currencyID string) (decimal.Decimal, error)
	Withdraw(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error
	Deposit(ctx context.Context, userID, currencyID string, amount decimal.Decimal) error
}

// ContributeRequest represents a request to contribute to a jackpot pool
type ContributeRequest struct {
	PoolID     string          // Pool ID to contribute to
	UserID     string          // User ID making the contribution
	Amount     decimal.Decimal // Contribution amount
	GameCode   string          // Game code
	SpinID     string          // Optional: spin/round ID to group contributions from the same spin
	TotalPools int            // Optional: total number of pools for this spin (for flush when complete)
}

// ClaimRequest represents a request to claim a jackpot pool
type ClaimRequest struct {
	PoolID    string          // Pool ID to claim from
	UserID    string          // User ID claiming the jackpot
	GameCode  string          // Game code
	InitValue decimal.Decimal // Initial pool value for claim calculation
}

// RewardProvider interface for jackpot/reward operations
type RewardProvider interface {
	Contribute(ctx context.Context, req *ContributeRequest) error
	Claim(ctx context.Context, req *ClaimRequest) (*JackpotClaim, error)
	GetPool(ctx context.Context, poolID string, initValue decimal.Decimal) (*JackpotPool, error)
}

// JackpotPool represents a jackpot pool response
type JackpotPool struct {
	PoolID    string          `json:"pool_id"`
	Amount    decimal.Decimal `json:"amount"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// JackpotClaim represents a jackpot claim response
type JackpotClaim struct {
	ClaimID    string          `json:"claim_id"`
	PoolID     string          `json:"pool_id"`
	UserID     string          `json:"user_id"`
	CurrencyID string          `json:"currency_id"`
	Amount     decimal.Decimal `json:"amount"`
	GameID     string          `json:"game_id"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// BetType represents the type of bet history to retrieve
type BetType string

const (
	BetTypeNormal   BetType = "normal"
	BetTypeFreeSpin BetType = "free_spin"
	BetTypeJackpot  BetType = "jackpot"
)

// LogProvider interface for logging game events
type LogProvider interface {
	LogSpin(ctx context.Context, log *SpinLog) (sessionID string, err error)
	LogJackpot(ctx context.Context, log *JackpotLog) (sessionID string, err error)
	GetBetHistory(ctx context.Context, query *BetHistoryQuery) (*BetHistoryResponse, error)
}

// SpinLog represents a spin log entry to be saved
type SpinLog struct {
	UserID     string      `json:"userId"`
	Username   string      `json:"username"`
	GameCode   string      `json:"gameCode"`
	BetAmount  float64     `json:"betAmount"`
	WinAmount  float64     `json:"winAmount"`
	SpinType   int         `json:"spinType"` // 0 = normal, 1 = free spin
	SpinResult interface{} `json:"spinResult"`
	Timestamp  time.Time   `json:"timestamp"`
}

// JackpotLog represents a jackpot log entry to be saved
type JackpotLog struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"` // Display name for history
	GameCode  string    `json:"gameCode"`
	Tier      string    `json:"tier"`      // "mini", "minor", "grand"
	BetAmount float64   `json:"betAmount"` // Bet amount when jackpot won
	WinAmount float64   `json:"winAmount"`
	Currency  string    `json:"currency"` // e.g. "USD", "VND"
	Timestamp time.Time `json:"timestamp"`
}

// BetHistoryQuery represents query parameters for bet history
type BetHistoryQuery struct {
	UserID   string  `json:"userId"`
	GameCode string  `json:"gameCode"`
	Type     BetType `json:"type"`
	Page     int     `json:"page"`
	Limit    int     `json:"limit"`
}

// Bet represents a single bet history item
type Bet struct {
	SessionID       string    `json:"sessionID"`
	Time            time.Time `json:"time"`
	TotalBet        float64   `json:"totalBet"`
	TotalWin        float64   `json:"totalWin"`
	TotalWinJackpot float64   `json:"totalWinJackpot,omitempty"`
	Username        *string   `json:"userName,omitempty"`
	IsFreeSpin      bool      `json:"isFreeSpin"`
	Reels           any       `json:"reels,omitempty"`
	WinLines        any       `json:"winLines,omitempty"`
	SubReel         any       `json:"subReel,omitempty"`
}

// BetHistoryResponse represents the response for bet history
type BetHistoryResponse struct {
	Total int   `json:"total"`
	Items []Bet `json:"items"`
}
