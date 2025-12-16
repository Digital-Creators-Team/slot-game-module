package game

import (
	"context"

	"github.com/shopspring/decimal"
)

// Module defines the interface that all game implementations must satisfy
//
// Flow: gameRoutes -> gameHandler -> gameService -> gameModule
//
// IMPORTANT: ModuleContext is set by middleware and available via game.MustFromContext(ctx)
// User info may be nil if no auth middleware was used - always check:
//
//   mc := game.MustFromContext(ctx)
//   if user := mc.User(); user != nil {
//       userID := user.ID()
//       username := user.Username()
//       currencyID := user.CurrencyID()
//   }
//   mc.GetLogger().Info().Msg("Playing spin")
//
// Example implementation:
//
//	type MyGameModule struct {
//		BaseModule              // Embed base module for common functionality
//		game.JackpotHandler     // Embed jackpot handler interface (optional)
//		rng *rand.Rand
//	}
//
//	func (m *MyGameModule) PlayNormalSpin(ctx context.Context, betMultiplier float32, cheatPayout interface{}) (*game.SpinResult, error) {
//		// ModuleContext is set by middleware
//		mc := game.MustFromContext(ctx)
//		
//		// User may be nil if no auth middleware
//		if user := mc.User(); user != nil {
//			userID := user.ID()
//			mc.GetLogger().Info().Str("user_id", userID).Msg("Playing spin")
//		}
//		// Your game logic here
//	}
type Module interface {
	// GetConfig returns the game configuration (must implement ConfigNormalizer)
	// ModuleContext is available via game.MustFromContext(ctx)
	GetConfig(ctx context.Context) (ConfigNormalizer, error)

	// PlayNormalSpin executes a normal spin with the given bet multiplier
	// cheatPayout can be used for testing/debugging to force specific outcomes
	// ModuleContext is ALWAYS available: ctx := game.MustFromContext(ctx)
	PlayNormalSpin(ctx context.Context, betMultiplier float32, cheatPayout interface{}) (*SpinResult, error)

	// PlayFreeSpin executes a free spin with the given bet multiplier
	// ModuleContext is ALWAYS available: ctx := game.MustFromContext(ctx)
	PlayFreeSpin(ctx context.Context, betMultiplier float32) (*SpinResult, error)

	// GenerateFreeSpins pre-generates all free spin results
	// ModuleContext is ALWAYS available: ctx := game.MustFromContext(ctx)
	GenerateFreeSpins(ctx context.Context, betMultiplier float32, count int) ([]*SpinResult, error)

	// GetGameCode returns the unique identifier for this game
	GetGameCode() string
}

// LifecycleHooks provides optional lifecycle hooks for game modules
type LifecycleHooks interface {
	// Init is called when the game module is initialized
	Init(ctx context.Context, cfg *Config) error

	// OnPlayerJoin is called when a player joins the game
	OnPlayerJoin(ctx context.Context, playerID string) error

	// OnPlayerLeave is called when a player leaves the game
	OnPlayerLeave(ctx context.Context, playerID string) error

	// OnShutdown is called when the game module is shutting down
	OnShutdown(ctx context.Context) error
}

// JackpotContribution represents a contribution to a jackpot pool
type JackpotContribution struct {
	PoolID string          // Pool ID (e.g., "game-code:mini", "game-code:grand")
	Amount decimal.Decimal // Contribution amount
}

// JackpotWin represents a jackpot win claim
type JackpotWin struct {
	PoolID    string          // Pool ID to claim from
	Tier      string          // Tier name (e.g., "mini", "minor", "grand") - for logging
	InitValue decimal.Decimal // Initial value for claim calculation
}

// JackpotHandler defines optional interface for custom jackpot logic
// If a game module implements this interface, it will be used instead of default jackpot logic
// This allows games to have custom jackpot rules (different number of pools, different contribution logic, etc.)
type JackpotHandler interface {
	// GetContributions returns the jackpot contributions for a spin
	// This is called when IsGetJackpot is false
	// Returns a list of pool contributions (can be empty if no contribution needed)
	GetContributions(ctx context.Context, spinResult *SpinResult, totalBet decimal.Decimal, gameConfig *Config) ([]JackpotContribution, error)

	// GetWin returns the jackpot win information for a spin
	// This is called when IsGetJackpot is true
	// Returns the win information (pool ID, tier, init value) or nil if no win
	GetWin(ctx context.Context, spinResult *SpinResult, totalBet decimal.Decimal, gameConfig *Config) (*JackpotWin, error)

	// GetPoolID returns the pool ID for SSE updates
	// This is used for jackpot SSE streaming
	// Can return multiple pool IDs if the game has multiple pools to display
	GetPoolID(ctx context.Context, gameCode string, betMultiplier float32, gameConfig *Config) ([]string, error)

	// GetInitialPoolValue returns the initial pool value for a given bet multiplier and pool ID
	// This is used for jackpot SSE streaming
	GetInitialPoolValue(ctx context.Context, poolID string, betMultiplier float32, gameConfig *Config) (decimal.Decimal, error)
}

// ModuleFactory is a function that creates a game module from a config
type ModuleFactory func(*Config) (Module, error)

// Registry holds registered module factories
type Registry struct {
	factories map[string]ModuleFactory
}

// NewRegistry creates a new module registry
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ModuleFactory),
	}
}

// Register registers a factory function for a game code
func (r *Registry) Register(gameCode string, factory ModuleFactory) {
	r.factories[gameCode] = factory
}

// Get returns the factory for a game code
func (r *Registry) Get(gameCode string) (ModuleFactory, bool) {
	factory, ok := r.factories[gameCode]
	return factory, ok
}

// GetAll returns all registered game codes
func (r *Registry) GetAll() []string {
	codes := make([]string, 0, len(r.factories))
	for code := range r.factories {
		codes = append(codes, code)
	}
	return codes
}

// DefaultRegistry is the default global registry
var DefaultRegistry = NewRegistry()

// Register registers a module factory in the default registry
func Register(gameCode string, factory ModuleFactory) {
	DefaultRegistry.Register(gameCode, factory)
}


