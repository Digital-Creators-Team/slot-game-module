package game

import "git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/providers"

// Alias provider interfaces/types from pkg/providers to avoid duplication.
type (
	StateProvider       = providers.StateProvider
	WalletProvider      = providers.WalletProvider
	RewardProvider      = providers.RewardProvider
	LogProvider         = providers.LogProvider
	JackpotPool         = providers.JackpotPool
	JackpotClaim        = providers.JackpotClaim
	SpinLog             = providers.SpinLog
	JackpotLog          = providers.JackpotLog
	BetHistoryQuery     = providers.BetHistoryQuery
	BetHistoryResponse  = providers.BetHistoryResponse
)
