package game

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Symbol represents a symbol on the game board
type Symbol struct {
	Symbol int `json:"symbol"`
	Value  int `json:"value"`
	Type   int `json:"type"`
}

type WinType int

const (
	WinTypeNoWin WinType = iota
	WinTypeNormalWin
	WinTypeBigWin
	WinTypeSuperWin
	WinTypeMegaWin
)

// Winline represents a winning line on the game board
type Winline struct {
	Symbol      int             `json:"symbol"`
	SameItem    int             `json:"sameItem"`
	LineID      int             `json:"lineId"`
	WinPosition []int           `json:"winPosition"`
	WinAmount   decimal.Decimal `json:"winAmount"`
	Multiplier  int             `json:"multiplier,omitempty"`
}

// SpinResult represents the result of a single spin
type SpinResult struct {
	Reels             [][]Symbol             `json:"reels,omitempty"`
	Winlines          []Winline              `json:"winlines,omitempty"`
	TotalWin          decimal.Decimal        `json:"totalWin,omitempty"`
	TotalBet          decimal.Decimal        `json:"totalBet,omitempty"`
	Multiplier        int                    `json:"multiplier,omitempty"`
	IsGetFreeSpin     *bool                  `json:"isGetFreeSpin,omitempty"`
	ResultFreeSpin    *int                   `json:"resultFreeSpin,omitempty"`
	FreeSpinBets      []*SpinResult          `json:"freeSpinBets,omitempty"`
	IsGetJackpot      *bool                  `json:"isGetJackpot,omitempty"`
	TotalWinJackpot   decimal.Decimal        `json:"totalWinJackpot,omitempty"`
	EndingBalance     decimal.Decimal        `json:"endingBalance,omitempty"`
	RemainingFreeSpin *int                   `json:"remainingFreeSpin,omitempty"`
	TotalWinFreeSpin  decimal.Decimal        `json:"totalWinFreeSpin,omitempty"`
	IsLastFreeSpin    *bool                  `json:"isLastFreeSpin,omitempty"`
	WinTitle          WinType                `json:"winTitle,omitempty"`
	SpinType          int                    `json:"spinType,omitempty"`  // 0 = normal, 1 = free spin
	SubReel           []int                  `json:"subReel,omitempty"`   // For games with sub reel (e.g., parrot treasure)
	ExtraData         map[string]interface{} `json:"extraData,omitempty"` // Custom data for game-specific use
}

// SpinResponse represents the API response for a spin
type SpinResponse struct {
	TotalWin          decimal.Decimal        `json:"totalWin,omitempty"`
	WinTitle          WinType                `json:"winTitle,omitempty"`
	ResultFreeSpin    *int                   `json:"resultFreeSpin,omitempty"`
	TotalWinJackpot   decimal.Decimal        `json:"totalWinJackpot,omitempty"`
	IsGetFreeSpin     *bool                  `json:"isGetFreeSpin,omitempty"`
	IsGetJackpot      *bool                  `json:"isGetJackpot,omitempty"`
	Reels             [][]Symbol             `json:"reels,omitempty"`
	Winlines          *[]Winline             `json:"winlines,omitempty"`
	TotalWinFreeSpin  decimal.Decimal        `json:"totalWinFreeSpin,omitempty"`
	RemainingFreeSpin *int                   `json:"remainingFreeSpin,omitempty"`
	IsLastFreeSpin    *bool                  `json:"isLastFreeSpin,omitempty"`
	EndingBalance     decimal.Decimal        `json:"endingBalance,omitempty"`
	TotalBet          decimal.Decimal        `json:"totalBet,omitempty"`
	Multiplier        int                    `json:"multiplier,omitempty"`
	SpinType          int                    `json:"spinType,omitempty"`
	JackpotPrize      decimal.Decimal        `json:"jackpotPrize,omitempty"`
	SubReel           []int                  `json:"subReel,omitempty"`
	ExtraData         map[string]interface{} `json:"extraData,omitempty"` // Custom data for game-specific use
}

// ToSpinResponse converts SpinResult to SpinResponse format
func (sr *SpinResult) ToSpinResponse() *SpinResponse {
	var winlines *[]Winline
	if len(sr.Winlines) > 0 {
		winlines = &sr.Winlines
	}

	return &SpinResponse{
		TotalWin:          sr.TotalWin,
		WinTitle:          sr.WinTitle,
		ResultFreeSpin:    sr.ResultFreeSpin,
		TotalWinJackpot:   sr.TotalWinJackpot,
		IsGetFreeSpin:     sr.IsGetFreeSpin,
		IsGetJackpot:      sr.IsGetJackpot,
		Reels:             sr.Reels,
		Winlines:          winlines,
		TotalWinFreeSpin:  sr.TotalWinFreeSpin,
		RemainingFreeSpin: sr.RemainingFreeSpin,
		IsLastFreeSpin:    sr.IsLastFreeSpin,
		EndingBalance:     sr.EndingBalance,
		TotalBet:          sr.TotalBet,
		Multiplier:        sr.Multiplier,
		SpinType:          sr.SpinType,
		JackpotPrize:      sr.TotalWinJackpot, // Alias for TotalWinJackpot
		SubReel:           sr.SubReel,
		ExtraData:         sr.ExtraData,
	}
}

// PlayerState represents the current state of a player in a game
type PlayerState struct {
	IsFreeSpin          bool                   `json:"isFreeSpin"`
	RemainingFreeSpin   int                    `json:"remainingFreeSpin"`
	TotalWinFreeSpin    decimal.Decimal        `json:"totalWinFreeSpin"`
	BetMultiplier       float32                `json:"betMultiplier"`
	FreeSpins           []*SpinResult          `json:"-"`
	PlayedFreeSpin      *int                   `json:"playedFreeSpin,omitempty"`
	IsLastFreeSpin      bool                   `json:"isLastFreeSpin"`
	SpinResult          *SpinResult            `json:"spinResult,omitempty"`
	SpinResultTriggerFG *SpinResult            `json:"spinResultTriggerFG,omitempty"`
	UpdatedAt           time.Time              `json:"updatedAt"`
	ExtraData           map[string]interface{} `json:"extraData,omitempty"` // Custom data for game-specific use
}

// ToJSON serializes PlayerState to JSON
func (p *PlayerState) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// PlayerStateFromJSON deserializes PlayerState from JSON
func PlayerStateFromJSON(data []byte) (*PlayerState, error) {
	var state PlayerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	// Initialize ExtraData if nil (for backward compatibility with old states)
	if state.ExtraData == nil {
		state.ExtraData = make(map[string]interface{})
	}
	return &state, nil
}

// NewPlayerState creates a new default player state
func NewPlayerState() *PlayerState {
	return &PlayerState{
		UpdatedAt: time.Now(),
	}
}

// Reset resets the player state to default values
func (p *PlayerState) Reset() {
	p.IsFreeSpin = false
	p.RemainingFreeSpin = 0
	p.TotalWinFreeSpin = decimal.Zero
	p.FreeSpins = nil
	p.PlayedFreeSpin = nil
	p.IsLastFreeSpin = false
	p.SpinResultTriggerFG = nil
	p.ExtraData = make(map[string]interface{})
	p.UpdatedAt = time.Now()
}

// Player represents player information
type Player struct {
	UserID   string          `json:"userID"`
	Username string          `json:"username"`
	Balance  decimal.Decimal `json:"balance"`
	Currency string          `json:"currency"`
}

// AuthorizeResponse represents the authorize game response
type AuthorizeResponse struct {
	LastState  interface{} `json:"lastState"`
	GameConfig interface{} `json:"gameConfig"`
	Player     Player      `json:"player"`
}

// Config holds game configuration
type Config struct {
	GameCode          string            `mapstructure:"game_code" json:"gameCode"`
	GameName          string            `mapstructure:"game_name" json:"gameName"`
	PayLine           int               `mapstructure:"pay_line" json:"payLine"`
	ReelRows          int               `mapstructure:"reel_rows" json:"reelRows"`
	ReelCols          int               `mapstructure:"reel_cols" json:"reelCols"`
	ReelSize          []int             `mapstructure:"reel_size" json:"reelSize"`
	BetMultipliers    []float32         `mapstructure:"bet_multipliers" json:"betMultipliers"`
	Symbols           []SymbolConfig    `mapstructure:"symbols" json:"symbols"`
	Reels             [][]int           `mapstructure:"reels" json:"reels"`
	Payouts           map[int][]float64 `mapstructure:"payouts" json:"payouts"`
	WildSymbol        int               `mapstructure:"wild_symbol" json:"wildSymbol"`
	WildSymbols       []int             `mapstructure:"wild_symbols" json:"wildSymbols"`
	ScatterSymbol     int               `mapstructure:"scatter_symbol" json:"scatterSymbol"`
	JackpotSymbol     int               `mapstructure:"jackpot_symbol" json:"jackpotSymbol"`
	ValidSymbols      []int             `mapstructure:"valid_symbols" json:"validSymbols"`
	Winlines          [][]int           `mapstructure:"winlines" json:"winlines"`
	PayTable          [][]float64       `mapstructure:"pay_table" json:"payTable"`
	ReelStrip         [][][]int         `mapstructure:"reel_strip" json:"reelStrip"`
	JackpotMultiplier int               `mapstructure:"jackpot_multiplier" json:"jackpotMultiplier"`
	FreeSpinConfig    FreeSpinConfig    `mapstructure:"free_spin" json:"freeSpin"`
	FreeSpinReward    map[string]int    `mapstructure:"free_spin_reward" json:"freeSpinReward"`
	MultiplierValue   []int             `mapstructure:"multiplier_value" json:"multiplierValue"`
	MultiplierWeight  []int             `mapstructure:"multiplier_weight" json:"multiplierWeight"`
	RTP               float64           `mapstructure:"rtp" json:"rtp"`
	Volatility        string            `mapstructure:"volatility" json:"volatility"`
}

// ConfigNormalizer exposes normalized config for responses.
type ConfigNormalizer interface {
	Normalize() map[string]interface{}
	// GetConfig returns the underlying *Config.
	// For *Config, returns itself.
	// For custom configs that embed Config, returns a pointer to the embedded Config.
	GetConfig() *Config
}

// Normalize converts Config to a response-friendly map.
func (c *Config) Normalize() map[string]interface{} {
	return map[string]interface{}{
		"payLine":           c.PayLine,
		"jackpotMultiplier": c.JackpotMultiplier,
		"reelSize":          c.ReelSize,
	}
}

// GetConfig returns the Config itself (implements ConfigNormalizer interface).
func (c *Config) GetConfig() *Config {
	return c
}

// GetConfigFromNormalizer extracts *Config from a ConfigNormalizer.
// Works with both *Config and custom config structs that embed Config.
// For custom config structs, it extracts the embedded Config field.
// This is a convenience function that calls the GetConfig() method on the normalizer.
func GetConfigFromNormalizer(normalizer ConfigNormalizer) (*Config, error) {
	if normalizer == nil {
		return nil, fmt.Errorf("ConfigNormalizer is nil")
	}
	cfg := normalizer.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("ConfigNormalizer.GetConfig() returned nil, config may not be properly embedded")
	}
	return cfg, nil
}

// SymbolConfig holds symbol configuration
type SymbolConfig struct {
	ID     int    `mapstructure:"id" json:"id"`
	Name   string `mapstructure:"name" json:"name"`
	Type   int    `mapstructure:"type" json:"type"`
	Weight int    `mapstructure:"weight" json:"weight"`
}

// JackpotConfig has been removed from base module.
// Games should define their own jackpot configuration structure in their config file
// and access it via custom config loading or by embedding in game.Config if needed.
// Use JackpotHandler interface to implement custom jackpot logic.

// FreeSpinConfig holds free spin configuration
type FreeSpinConfig struct {
	TriggerCount int   `mapstructure:"trigger_count" json:"triggerCount"`
	SpinCount    int   `mapstructure:"spin_count" json:"spinCount"`
	Multipliers  []int `mapstructure:"multipliers" json:"multipliers"`
	RetriggerMax int   `mapstructure:"retrigger_max" json:"retriggerMax"`
}

// CheatPosition represents cheat positions for testing
type CheatPosition struct {
	Sym []int `json:"symbols"`
	Pos []int `json:"positions"`
}

// CheatPayout represents cheat payout configuration for testing
type CheatPayout struct {
	TriggerWinMode  *WinType       `json:"triggerWinMode,omitempty"`
	Position        *CheatPosition `json:"cheatPosition,omitempty"`
	TriggerFreeGame *bool          `json:"triggerFreeGame,omitempty"`
	TriggerJackpot  *bool          `json:"triggerJackpot,omitempty"`
}

// SpinRequest represents a spin request DTO
type SpinRequest struct {
	BetMultiplier float32                `json:"betMultiplier" binding:"required"`
	GameCode      string                 `json:"gameCode" binding:"required"`
	CheatPayout   *CheatPayout           `json:"cheatPayout,omitempty"`
	ExtraData     map[string]interface{} `json:"extraData,omitempty"` // Custom data for game-specific use
}
