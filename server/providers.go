package server

import "github.com/Digital-Creators-Team/slot-game-module/pkg/providers"

// Re-export provider interfaces/types from pkg/providers to keep a single source of truth.
type (
	StateProvider      = providers.StateProvider
	WalletProvider     = providers.WalletProvider
	RewardProvider     = providers.RewardProvider
	LogProvider        = providers.LogProvider
	JackpotPool        = providers.JackpotPool
	JackpotClaim       = providers.JackpotClaim
	BetType            = providers.BetType
	SpinLog            = providers.SpinLog
	JackpotLog         = providers.JackpotLog
	BetHistoryQuery    = providers.BetHistoryQuery
	Bet                = providers.Bet
	BetHistoryResponse = providers.BetHistoryResponse
)

const (
	BetTypeNormal   = providers.BetTypeNormal
	BetTypeFreeSpin = providers.BetTypeFreeSpin
	BetTypeJackpot  = providers.BetTypeJackpot
)
