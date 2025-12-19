package game

import (
	"context"

	"github.com/rs/zerolog"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/providers"
)

// ModuleContext provides access to dependencies and services for game modules
// This is set by middleware and available in game module methods
// Use game.MustFromContext(ctx) to get it (panics if not found)
// Use game.FromContext(ctx) to get it safely (returns nil if not found)
type ModuleContext struct {
	// User information (may be nil if no auth middleware)
	user *User

	// Logger for logging (always available)
	Logger zerolog.Logger

	// Providers (optional, can be nil if not needed)
	// Use helper functions (StateProvider(), etc.) for type-safe access
	stateProvider  interface{} // providers.StateProvider
	walletProvider interface{} // providers.WalletProvider
	rewardProvider interface{} // providers.RewardProvider
	logProvider    interface{} // providers.LogProvider

	// Player state (set by game service before calling game module methods)
	// Endusers can access and modify this in their game module implementations
	playerState *PlayerState

	// Extra data from spin request (set by game service before calling game module methods)
	// Endusers can access this in their game module implementations
	spinRequestExtraData map[string]interface{}
}

// NewModuleContext creates a new ModuleContext
// This is used internally by the server package
func NewModuleContext(user *User, logger zerolog.Logger, stateProvider, walletProvider, rewardProvider, logProvider interface{}) *ModuleContext {
	return &ModuleContext{
		user:          user,
		Logger:        logger,
		stateProvider:  stateProvider,
		walletProvider: walletProvider,
		rewardProvider: rewardProvider,
		logProvider:    logProvider,
	}
}

// User returns the current user information
// Returns nil if no auth middleware was used or user not authenticated
// Always check for nil before using:
//   if user := mc.User(); user != nil {
//       userID := user.ID()
//   }
func (mc *ModuleContext) User() *User {
	return mc.user
}

// GetUser is an alias for User() for convenience
func (mc *ModuleContext) GetUser() *User {
	return mc.user
}

// HasUser returns true if user information is available
func (mc *ModuleContext) HasUser() bool {
	return mc.user != nil
}

// RequireUser returns the current user or panics if no user is available.
// Use when the operation strictly requires authentication.
func (mc *ModuleContext) RequireUser() *User {
	if mc.user == nil {
		panic("user is required but not present in ModuleContext (ensure auth middleware is applied)")
	}
	return mc.user
}

// Logger returns the logger from context
// This is a convenience function to avoid nil checks
func (mc *ModuleContext) GetLogger() zerolog.Logger {
	return mc.Logger
}

// StateProvider returns the state provider if available (type-safe, no casting needed)
// Returns nil if not available
func (mc *ModuleContext) StateProvider() providers.StateProvider {
	if mc.stateProvider == nil {
		return nil
	}
	if sp, ok := mc.stateProvider.(providers.StateProvider); ok {
		return sp
	}
	return nil
}

// WalletProvider returns the wallet provider if available (type-safe, no casting needed)
// Returns nil if not available
func (mc *ModuleContext) WalletProvider() providers.WalletProvider {
	if mc.walletProvider == nil {
		return nil
	}
	if wp, ok := mc.walletProvider.(providers.WalletProvider); ok {
		return wp
	}
	return nil
}

// RewardProvider returns the reward provider if available (type-safe, no casting needed)
// Returns nil if not available
func (mc *ModuleContext) RewardProvider() providers.RewardProvider {
	if mc.rewardProvider == nil {
		return nil
	}
	if rp, ok := mc.rewardProvider.(providers.RewardProvider); ok {
		return rp
	}
	return nil
}

// LogProvider returns the log provider if available (type-safe, no casting needed)
// Returns nil if not available
func (mc *ModuleContext) LogProvider() providers.LogProvider {
	if mc.logProvider == nil {
		return nil
	}
	if lp, ok := mc.logProvider.(providers.LogProvider); ok {
		return lp
	}
	return nil
}

// GetStateProvider returns the state provider as interface{} (for backward compatibility)
// Prefer using StateProvider() for type-safe access
func (mc *ModuleContext) GetStateProvider() interface{} {
	return mc.stateProvider
}

// GetWalletProvider returns the wallet provider as interface{} (for backward compatibility)
// Prefer using WalletProvider() for type-safe access
func (mc *ModuleContext) GetWalletProvider() interface{} {
	return mc.walletProvider
}

// GetRewardProvider returns the reward provider as interface{} (for backward compatibility)
// Prefer using RewardProvider() for type-safe access
func (mc *ModuleContext) GetRewardProvider() interface{} {
	return mc.rewardProvider
}

// GetLogProvider returns the log provider as interface{} (for backward compatibility)
// Prefer using LogProvider() for type-safe access
func (mc *ModuleContext) GetLogProvider() interface{} {
	return mc.logProvider
}

// GetPlayerState returns the current player state
// This is set by the game service before calling game module methods (PlayNormalSpin, PlayFreeSpin, etc.)
// Returns nil if not set (should not happen in normal game module methods)
// Endusers can use this to read and modify player state in their game implementations
//
// Since playerState is a pointer, direct modifications are automatically reflected.
// No need to call any update method - just modify the state directly.
//
// Example usage in PlayNormalSpin:
//   mc := game.MustFromContext(ctx)
//   playerState := mc.GetPlayerState()
//   if playerState != nil {
//       // Read current state
//       isFreeSpin := playerState.IsFreeSpin
//       // Modify state directly - changes are automatically reflected
//       playerState.ExtraData["customField"] = "customValue"
//       playerState.BetMultiplier = 2.0
//   }
func (mc *ModuleContext) GetPlayerState() *PlayerState {
	return mc.playerState
}

// setPlayerState sets the player state in the context
// This is used internally by the game service to make player state available to game modules
func (mc *ModuleContext) setPlayerState(state *PlayerState) {
	mc.playerState = state
}

// SetPlayerStateForModule sets the player state in the ModuleContext
// This is a helper function for internal use by the game service
// Endusers should use GetPlayerState() to access and modify the state
func SetPlayerStateForModule(ctx context.Context, state *PlayerState) {
	if mc := FromContext(ctx); mc != nil {
		mc.setPlayerState(state)
	}
}

// setSpinRequestExtraData sets the extra data from spin request in the context
// This is used internally by the game service
func (mc *ModuleContext) setSpinRequestExtraData(extraData map[string]interface{}) {
	mc.spinRequestExtraData = extraData
}

// GetSpinRequestExtraData returns the extra data from the spin request
// This is set by the game service before calling game module methods (PlayNormalSpin, PlayFreeSpin, etc.)
// Returns nil if not set
// Endusers can use this to access custom data sent in the spin request
//
// Example usage in PlayNormalSpin:
//   mc := game.MustFromContext(ctx)
//   if extraData := mc.GetSpinRequestExtraData(); extraData != nil {
//       customValue := extraData["customField"]
//   }
func (mc *ModuleContext) GetSpinRequestExtraData() map[string]interface{} {
	return mc.spinRequestExtraData
}

// SetSpinRequestExtraDataForModule sets the extra data from spin request in the ModuleContext
// This is a helper function for internal use by the game service
// Endusers should use GetSpinRequestExtraData() to access the data
func SetSpinRequestExtraDataForModule(ctx context.Context, extraData map[string]interface{}) {
	if mc := FromContext(ctx); mc != nil {
		mc.setSpinRequestExtraData(extraData)
	}
}

// WithContext attaches ModuleContext to a context
func WithContext(ctx context.Context, mc *ModuleContext) context.Context {
	return context.WithValue(ctx, contextKeyModuleContext, mc)
}

// FromContext extracts ModuleContext from context
// Returns nil if not found (should not happen in normal game module methods)
// Use MustFromContext() in game modules for guaranteed access
func FromContext(ctx context.Context) *ModuleContext {
	if mc, ok := ctx.Value(contextKeyModuleContext).(*ModuleContext); ok {
		return mc
	}
	return nil
}

// MustFromContext extracts ModuleContext from context, panics if not found
// Use this in game module methods - ModuleContext is always available
// Example: ctx := game.MustFromContext(ctx); userID := ctx.User().ID()
func MustFromContext(ctx context.Context) *ModuleContext {
	mc := FromContext(ctx)
	if mc == nil {
		panic("ModuleContext not found in context - this should not happen in game module methods")
	}
	return mc
}

type contextKey string

const contextKeyModuleContext contextKey = "module_context"
