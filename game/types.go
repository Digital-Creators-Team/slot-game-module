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
	Reels             [][]Symbol      `json:"reels,omitempty"`
	Winlines          []Winline       `json:"winlines,omitempty"`
	TotalWin          decimal.Decimal `json:"totalWin,omitempty"`
	TotalBet          decimal.Decimal `json:"totalBet,omitempty"`
	Multiplier        int             `json:"multiplier,omitempty"`
	IsGetFreeSpin     *bool           `json:"isGetFreeSpin,omitempty"`
	ResultFreeSpin    *int            `json:"resultFreeSpin,omitempty"`
	FreeSpinBets      []*SpinResult   `json:"freeSpinBets,omitempty"`
	IsGetJackpot      *bool           `json:"isGetJackpot,omitempty"`
	TotalWinJackpot   decimal.Decimal `json:"totalWinJackpot,omitempty"`
	EndingBalance     decimal.Decimal `json:"endingBalance,omitempty"`
	RemainingFreeSpin *int            `json:"remainingFreeSpin,omitempty"`
	TotalWinFreeSpin  decimal.Decimal `json:"totalWinFreeSpin,omitempty"`
	IsLastFreeSpin    *bool           `json:"isLastFreeSpin,omitempty"`
	WinTitle          string          `json:"winTitle,omitempty"`
	SpinType          int             `json:"spinType,omitempty"` // 0 = normal, 1 = free spin
	SubReel           []int           `json:"subReel,omitempty"`  // For games with sub reel (e.g., parrot treasure)
}

// SpinResponse represents the API response for a spin
type SpinResponse struct {
	TotalWin          decimal.Decimal `json:"totalWin,omitempty"`
	WinTitle          string          `json:"winTitle,omitempty"`
	ResultFreeSpin    *int            `json:"resultFreeSpin,omitempty"`
	TotalWinJackpot   decimal.Decimal `json:"totalWinJackpot,omitempty"`
	IsGetFreeSpin     *bool           `json:"isGetFreeSpin,omitempty"`
	IsGetJackpot      *bool           `json:"isGetJackpot,omitempty"`
	Reels             [][]Symbol      `json:"reels,omitempty"`
	Winlines          *[]Winline      `json:"winlines,omitempty"`
	TotalWinFreeSpin  decimal.Decimal `json:"totalWinFreeSpin,omitempty"`
	RemainingFreeSpin *int            `json:"remainingFreeSpin,omitempty"`
	IsLastFreeSpin    *bool           `json:"isLastFreeSpin,omitempty"`
	EndingBalance     decimal.Decimal `json:"endingBalance,omitempty"`
	TotalBet          decimal.Decimal `json:"totalBet,omitempty"`
	Multiplier        int             `json:"multiplier,omitempty"`
	SpinType          int             `json:"spinType,omitempty"`
	JackpotPrize      decimal.Decimal `json:"jackpotPrize,omitempty"`
	SubReel           []int           `json:"subReel,omitempty"`
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
	}
}

// PlayerState represents the current state of a player in a game
type PlayerState struct {
	IsFreeSpin          bool            `json:"isFreeSpin"`
	RemainingFreeSpin   int             `json:"remainingFreeSpin"`
	TotalWinFreeSpin    decimal.Decimal `json:"totalWinFreeSpin"`
	BetMultiplier       float32         `json:"betMultiplier"`
	FreeSpins           []*SpinResult   `json:"freeSpins,omitempty"`
	PlayedFreeSpin      *int            `json:"playedFreeSpin,omitempty"`
	IsLastFreeSpin      bool            `json:"isLastFreeSpin"`
	SpinResult          *SpinResult     `json:"spinResult,omitempty"`
	SpinResultTriggerFG *SpinResult     `json:"spinResultTriggerFG,omitempty"`
	UpdatedAt           time.Time       `json:"updatedAt"`
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
	return &state, nil
}

// NewPlayerState creates a new default player state
func NewPlayerState() *PlayerState {
	return &PlayerState{
		IsFreeSpin:        false,
		RemainingFreeSpin: 0,
		TotalWinFreeSpin:  decimal.Zero,
		BetMultiplier:     1.0,
		UpdatedAt:         time.Now(),
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
}

// Normalize converts Config to a response-friendly map.
func (c *Config) Normalize() map[string]interface{} {
	return map[string]interface{}{
		"payLine":           c.PayLine,
		"jackpotMultiplier": c.JackpotMultiplier,
		"reelSize":          c.ReelSize,
	}
}

// GetConfigFromNormalizer extracts *Config from a ConfigNormalizer.
// Simple type assertion - works when ConfigNormalizer is actually *Config.
// For custom config structs that embed Config, store the full struct in your module
// and access fields directly (e.g., m.gameConfig.PayLine instead of cfg.PayLine).
func GetConfigFromNormalizer(normalizer ConfigNormalizer) (*Config, error) {
	cfg, ok := normalizer.(*Config)
	if !ok {
		return nil, fmt.Errorf("ConfigNormalizer is not *Config, got %T. Store full config struct in module instead", normalizer)
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
	Sym []int `json:"sym"`
	Pos []int `json:"pos"`
}

// CheatPayout represents cheat payout configuration for testing
type CheatPayout struct {
	TriggerWinMode  *string        `json:"triggerWinMode,omitempty"`
	Position        *CheatPosition `json:"cheatPosition,omitempty"`
	TriggerFreeGame *bool          `json:"triggerFreeGame,omitempty"`
	TriggerJackpot  *bool          `json:"triggerJackpot,omitempty"`
}

// SpinRequest represents a spin request DTO
type SpinRequest struct {
	BetMultiplier float32      `json:"betMultiplier" binding:"required"`
	GameCode      string       `json:"gameCode" binding:"required"`
	CheatPayout   *CheatPayout `json:"cheatPayout,omitempty"`
}
