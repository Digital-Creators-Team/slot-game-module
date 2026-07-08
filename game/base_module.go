package game

import (
	"context"
	"fmt"
)

// BaseModule provides a base implementation that game modules can embed
// This makes it easier to implement the Module interface by providing
// default implementations that can be overridden
//
// Usage:
//
//	type MyGameModule struct {
//		BaseModule              // Embed for common functionality
//		// game.JackpotHandler   // Uncomment to implement custom jackpot
//		rng *rand.Rand
//	}
//
//	func NewMyGameModule(configPath string) (*MyGameModule, error) {
//		module, err := NewBaseModule("my-game", configPath)
//		if err != nil {
//			return nil, err
//		}
//		return &MyGameModule{
//			BaseModule: *module,
//			rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
//		}, nil
//	}
type BaseModule struct {
	// GameCode is the unique identifier for this game
	GameCode string

	// Config holds the game configuration
	Config ConfigNormalizer
}

// NewBaseModule creates a new BaseModule with config loaded from file
// This is a helper to reduce boilerplate in game module constructors
func NewBaseModule(gameCode, configPath string) (*BaseModule, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return &BaseModule{
		GameCode: gameCode,
		Config:   cfg,
	}, nil
}

// GetConfig returns the game configuration
// Can be overridden if you need custom config logic
func (b *BaseModule) GetConfig(ctx context.Context) (ConfigNormalizer, error) {
	if b.Config == nil {
		return nil, fmt.Errorf("config not set")
	}
	return b.Config, nil
}

// GetGameCode returns the unique identifier for this game
// Can be overridden if you need custom game code logic
func (b *BaseModule) GetGameCode() string {
	return b.GameCode
}

// PlayNormalSpin is a placeholder that must be overridden
// This ensures that game modules implement their own spin logic
func (b *BaseModule) PlayNormalSpin(ctx context.Context, betMultiplier float32, cheatPayout interface{}) (*SpinResult, error) {
	return nil, fmt.Errorf("PlayNormalSpin must be implemented by game module")
}

// PlayFreeSpin is a placeholder that must be overridden
// This ensures that game modules implement their own free spin logic
func (b *BaseModule) PlayFreeSpin(ctx context.Context, betMultiplier float32) (*SpinResult, error) {
	return nil, fmt.Errorf("PlayFreeSpin must be implemented by game module")
}

// GenerateFreeSpins is a placeholder that must be overridden
// This ensures that game modules implement their own free spin generation logic
func (b *BaseModule) GenerateFreeSpins(ctx context.Context, betMultiplier float32, count int) ([]*SpinResult, error) {
	return nil, fmt.Errorf("GenerateFreeSpins must be implemented by game module")
}
