package jackpot

import (
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/pkg/providers"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// PoolConfig describes a jackpot pool.
type PoolConfig struct {
	ID   string          // e.g. "game:mini"
	Init decimal.Decimal // starting value
	Prog decimal.Decimal // progressive rate (percentage of total bet, e.g. 0.01 for 1%)
}

// Contribution represents a contribution to a pool.
type Contribution struct {
	PoolID string
	Amount decimal.Decimal
}

// Update represents a pool value update (e.g. from Kafka or manual).
type Update struct {
	PoolID    string
	Amount    decimal.Decimal
	Timestamp time.Time
	SpinID    string // Optional: spin/round ID to group updates from the same spin
	TotalPools int   // Optional: total number of pools for this spin (0 = unknown, flush on timeout only)
}

// RewardProvider aliases the shared providers.RewardProvider (includes Contribute/Claim/GetPool).
type RewardProvider = providers.RewardProvider

// ServiceConfig configures the jackpot service.
type ServiceConfig struct {
	// BroadcastInterval controls how often buffered updates are flushed to listeners.
	BroadcastInterval time.Duration

	// Logger is optional; if zero value, a no-op logger is used.
	Logger zerolog.Logger

	// RewardProvider is optional; if set, ContributeAndApply will persist via this provider.
	RewardProvider RewardProvider

	// GameCode is optional; used when calling RewardStore.Contribute.
	GameCode string
}
