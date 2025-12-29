package server

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/providers"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// SpinService defines the minimal contract for executing a spin flow.
type SpinService interface {
	ExecuteSpin(ctx context.Context, req *SpinServiceRequest) (*SpinServiceResponse, error)
}

// GameService orchestrates the full spin flow
//
// Flow: gameRoutes -> gameHandler -> gameService -> gameModule
//
// The service:
// 1. Validates requests
// 2. Manages wallet transactions (withdraw/deposit)
// 3. Calls game module for spin logic
// 4. Handles jackpot contributions/wins
// 5. Updates player state
// 6. Logs game events
//
// Game modules receive ModuleContext in the context, allowing them to:
// - Access logger
// - Access providers (if needed for custom logic)
// - Access other services
type GameService struct {
	gameModule     game.Module
	stateProvider  providers.StateProvider
	walletProvider providers.WalletProvider
	rewardProvider providers.RewardProvider
	logProvider    providers.LogProvider
	logger         zerolog.Logger
}

// NewGameService creates a new default game service
func NewGameService(
	gameModule game.Module,
	stateProvider providers.StateProvider,
	walletProvider providers.WalletProvider,
	rewardProvider providers.RewardProvider,
	logProvider providers.LogProvider,
	logger zerolog.Logger,
) *GameService {
	return &GameService{
		gameModule:     gameModule,
		stateProvider:  stateProvider,
		walletProvider: walletProvider,
		rewardProvider: rewardProvider,
		logProvider:    logProvider,
		logger:         logger.With().Str("service", "game").Logger(),
	}
}

// SpinRequest represents a spin request
type SpinServiceRequest struct {
	UserID        string
	Username      string
	CurrencyID    string
	BetMultiplier float32
	CheatPayout   *game.CheatPayout
	ExtraData     map[string]interface{}
}

// SpinResponse represents a spin response
type SpinServiceResponse struct {
	SpinResult    *game.SpinResult
	SessionID     string
	EndingBalance decimal.Decimal
}

// ExecuteSpin orchestrates the entire spin flow
//
// Flow:
// 1. Validate request
// 2. Get game config
// 3. Load player state
// 4. Calculate total bet
// 5. Execute spin (normal or free)
// 6. Contribute to jackpot (progressive amount before spin)
// 7. Claim jackpot if won
// 8. Update wallet
// 9. Log spin
// 10. Save player state
func (s *GameService) ExecuteSpin(ctx context.Context, req *SpinServiceRequest) (*SpinServiceResponse, error) {
	// 1. Validate input
	if err := s.validateSpinRequest(req); err != nil {
		return nil, err
	}

	gameCode := s.gameModule.GetGameCode()

	// 2. Get game config
	cfg, err := s.gameModule.GetConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternalServerError, "failed to get game config")
	}
	gameConfig, err := game.GetConfigFromNormalizer(cfg)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternalServerError, "invalid game config type")
	}

	// 3. Load player state
	playerState, err := s.getPlayerState(ctx, req.UserID, gameCode)
	if err != nil {
		return nil, err
	}

	// Set player state in ModuleContext so endusers can access and modify it
	// Since playerState is a pointer, modifications by endusers are automatically reflected
	game.SetPlayerStateForModule(ctx, playerState)

	// Set extra data from spin request in ModuleContext so endusers can access it
	game.SetSpinRequestExtraDataForModule(ctx, req.ExtraData)

	// 4. Calculate total bet
	totalBet := decimal.NewFromFloat32(req.BetMultiplier).Mul(decimal.NewFromInt(int64(gameConfig.PayLine)))

	// 5. Determine spin type and execute
	var spinResult *game.SpinResult
	var sessionID string

	if playerState.IsFreeSpin && playerState.RemainingFreeSpin > 0 {
		// Execute free spin
		spinResult, err = s.executeFreeSpin(ctx, playerState, req.BetMultiplier)
		if err != nil {
			return nil, err
		}
	} else {
		// Execute normal spin
		spinResult, err = s.executeNormalSpin(ctx, req, playerState, gameConfig, totalBet)
		if err != nil {
			return nil, err
		}
	}

	// 6. Process jackpot win (if any)
	if spinResult.IsGetJackpot != nil && *spinResult.IsGetJackpot {
		// Claim jackpot
		if err := s.processJackpotWin(ctx, spinResult, req.UserID, req.Username, gameCode, req.CurrencyID, gameConfig, totalBet); err != nil {
			s.logger.Error().Err(err).Msg("Failed to process jackpot win")
		}
	}

	// Update timestamp before saving
	// Note: Since playerState is a pointer, any modifications by endusers in PlayNormalSpin/PlayFreeSpin
	// are automatically reflected here - no need to get state from ModuleContext again
	playerState.UpdatedAt = time.Now()

	// 7. Get ending balance
	var balance decimal.Decimal
	if s.walletProvider != nil {
		balance, err = s.walletProvider.GetBalance(ctx, req.UserID, req.CurrencyID)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to get ending balance")
			balance = decimal.Zero
		}
	} else {
		s.logger.Warn().Msg("Wallet provider not configured, ending balance will be zero")
		balance = decimal.Zero
	}

	spinResult.EndingBalance = balance

	// 8. Log spin
	if s.logProvider != nil {
		if spinResult.IsGetJackpot != nil && *spinResult.IsGetJackpot {
			sessionID, err = s.logProvider.LogJackpot(ctx, &JackpotLog{
				UserID:          req.UserID,
				Username:        req.Username,
				GameCode:        gameCode,
				Tier:            *spinResult.JackpotType,
				BetAmount:       totalBet.InexactFloat64(),
				WinAmount:       spinResult.TotalWin.InexactFloat64(),
				TotalWinJackpot: spinResult.TotalWinJackpot.InexactFloat64(),
				SpinType:        spinResult.SpinType,
				Currency:        req.CurrencyID,
				SpinResult:      spinResult,
				Timestamp:       time.Now().UTC(),
			})
			if err != nil {
				s.logger.Error().Err(err).Msg("Failed to log jackpot")
			}
		} else {
			sessionID, err = s.logProvider.LogSpin(ctx, &SpinLog{
				UserID:     req.UserID,
				Username:   req.Username,
				GameCode:   gameCode,
				BetAmount:  spinResult.TotalBet.InexactFloat64(),
				WinAmount:  spinResult.TotalWin.InexactFloat64(),
				SpinType:   spinResult.SpinType,
				SpinResult: spinResult,
				Timestamp:  time.Now().UTC(),
			})
			if err != nil {
				s.logger.Error().Err(err).Msg("Failed to log spin")
			}
		}

	}

	// 8. Save player state
	if err := s.savePlayerState(ctx, req.UserID, gameCode, playerState); err != nil {
		return nil, err
	}

	return &SpinServiceResponse{
		SpinResult:    spinResult,
		SessionID:     sessionID,
		EndingBalance: balance,
	}, nil
}

// validateSpinRequest validates the spin request
func (s *GameService) validateSpinRequest(req *SpinServiceRequest) error {
	if req.BetMultiplier <= 0 {
		return errors.New(errors.ErrInvalidRequest, "bet multiplier must be greater than 0")
	}
	if req.UserID == "" {
		return errors.New(errors.ErrInvalidRequest, "user ID is required")
	}
	if req.CurrencyID == "" {
		return errors.New(errors.ErrInvalidRequest, "currency ID is required")
	}
	return nil
}

// getPlayerState gets player state from provider
func (s *GameService) getPlayerState(ctx context.Context, userID, gameCode string) (*game.PlayerState, error) {
	stateInterface, err := s.stateProvider.GetPlayerState(ctx, userID, gameCode)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrPlayerStateError, "failed to get player state")
	}

	// Convert interface to PlayerState
	if ps, ok := stateInterface.(*game.PlayerState); ok {
		return ps, nil
	}

	// Return default state if conversion fails
	return game.NewPlayerState(), nil
}

// savePlayerState saves player state
func (s *GameService) savePlayerState(ctx context.Context, userID, gameCode string, state *game.PlayerState) error {
	if err := s.stateProvider.SavePlayerState(ctx, userID, gameCode, state); err != nil {
		return errors.Wrap(err, errors.ErrPlayerStateError, "failed to save player state")
	}
	return nil
}

// executeNormalSpin executes a normal spin with full flow
func (s *GameService) executeNormalSpin(
	ctx context.Context,
	req *SpinServiceRequest,
	playerState *game.PlayerState,
	gameConfig *game.Config,
	totalBet decimal.Decimal,
) (*game.SpinResult, error) {
	gameCode := s.gameModule.GetGameCode()

	// 1. Withdraw bet from wallet
	if s.walletProvider == nil {
		return nil, errors.New(errors.ErrInternalServerError, "wallet provider not configured")
	}
	if err := s.walletProvider.Withdraw(ctx, req.UserID, req.CurrencyID, totalBet); err != nil {
		return nil, errors.Wrap(err, errors.ErrInsufficientBalance, "failed to withdraw bet")
	}

	// 2. Execute spin
	spinResult, err := s.gameModule.PlayNormalSpin(ctx, req.BetMultiplier, req.CheatPayout)
	if err != nil {
		// Try to refund the bet (walletProvider is already checked above)
		if s.walletProvider != nil {
			if refundErr := s.walletProvider.Deposit(ctx, req.UserID, req.CurrencyID, totalBet); refundErr != nil {
				s.logger.Error().Err(refundErr).Msg("Failed to refund bet after spin error")
			}
		}
		return nil, errors.Wrap(err, errors.ErrInternalServerError, "failed to execute spin")
	}

	// 3. Contribute to jackpot pools (before claiming)
	// Progressive amount is calculated before spin, so we contribute first
	if err := s.contributeToJackpot(ctx, req.UserID, gameCode, gameConfig, totalBet, spinResult); err != nil {
		s.logger.Error().Err(err).Msg("Failed to contribute to jackpot")
	}

	// 5. Deposit winnings to wallet
	if spinResult.TotalWin.GreaterThan(decimal.Zero) {
		if s.walletProvider == nil {
			return nil, errors.New(errors.ErrInternalServerError, "wallet provider not configured")
		}
		if err := s.walletProvider.Deposit(ctx, req.UserID, req.CurrencyID, spinResult.TotalWin); err != nil {
			return nil, errors.Wrap(err, errors.ErrWalletError, "failed to deposit winnings")
		}
	}

	// Default state
	playerState.BetMultiplier = req.BetMultiplier
	playerState.SpinResult = spinResult

	if !playerState.IsFreeSpin {
		// Reset player state
		playerState.Reset()
	}

	// 6. Update player state if free spins triggered
	// Note: playerState is a pointer, so modifications by endusers in PlayNormalSpin are already reflected
	if spinResult.IsGetFreeSpin != nil && *spinResult.IsGetFreeSpin {
		if spinResult.ResultFreeSpin != nil && *spinResult.ResultFreeSpin > 0 {
			playerState.IsFreeSpin = true
			playerState.RemainingFreeSpin = *spinResult.ResultFreeSpin
			playerState.TotalWinFreeSpin = decimal.Zero
			playerState.SpinResultTriggerFG = spinResult
			playedCount := 0
			playerState.PlayedFreeSpin = &playedCount
			playerState.IsLastFreeSpin = false

			// Only store in the db
			playerState.FreeSpins = spinResult.FreeSpinBets
			spinResult.FreeSpinBets = nil

		}
	}

	return spinResult, nil
}

// executeFreeSpin executes a free spin
func (s *GameService) executeFreeSpin(
	ctx context.Context,
	playerState *game.PlayerState,
	betMultiplier float32,
) (*game.SpinResult, error) {
	// Get the next pre-generated free spin result
	// Note: playerState is a pointer, so modifications by endusers are automatically reflected
	if playerState.PlayedFreeSpin == nil {
		playedCount := 0
		playerState.PlayedFreeSpin = &playedCount
	}

	playedIndex := *playerState.PlayedFreeSpin
	if playedIndex >= len(playerState.FreeSpins) {
		return nil, errors.New(errors.ErrInvalidRequest, "no more free spins available")
	}

	spinResult := playerState.FreeSpins[playedIndex]

	// Update player state
	*playerState.PlayedFreeSpin++
	playerState.RemainingFreeSpin--
	playerState.TotalWinFreeSpin = playerState.TotalWinFreeSpin.Add(spinResult.TotalWin)

	// Update spin result with free spin info
	spinResult.RemainingFreeSpin = &playerState.RemainingFreeSpin
	totalWinFS := playerState.TotalWinFreeSpin
	spinResult.TotalWinFreeSpin = totalWinFS

	// Check if this is the last free spin
	if playerState.RemainingFreeSpin == 0 {
		isLast := true
		spinResult.IsLastFreeSpin = &isLast
		playerState.IsLastFreeSpin = true

		s.logger.Info().
			Float64("total_win", playerState.TotalWinFreeSpin.InexactFloat64()).
			Msg("Free spins completed")

		// Exit free spin
		playerState.IsFreeSpin = false

	}

	return spinResult, nil
}

// contributeToJackpot contributes to jackpot pools
// Uses custom JackpotHandler if implemented by game module, otherwise uses default logic
func (s *GameService) contributeToJackpot(ctx context.Context, userID, gameCode string, gameConfig *game.Config, totalBet decimal.Decimal, spinResult *game.SpinResult) error {
	if s.rewardProvider == nil {
		return nil
	}

	// Check if game module implements custom jackpot handler
	if handler, ok := s.gameModule.(game.JackpotHandler); ok {
		// Use custom jackpot handler
		contributions, err := handler.GetContributions(ctx, spinResult, totalBet)
		if err != nil {
			return fmt.Errorf("failed to get jackpot contributions: %w", err)
		}

		if len(contributions) == 0 {
			return nil
		}

		// Generate spin_id for this contribution batch
		// All contributions from the same spin will share the same spin_id
		spinID := uuid.New().String()
		totalPools := len(contributions)

		// Process each contribution with the same spin_id and total_pools
		for _, contrib := range contributions {
			if err := s.rewardProvider.Contribute(ctx, &providers.ContributeRequest{
				PoolID:     contrib.PoolID,
				UserID:     userID,
				Amount:     contrib.Amount,
				GameCode:   gameCode,
				SpinID:     spinID,
				TotalPools: totalPools,
			}); err != nil {
				s.logger.Error().Err(err).Str("pool", contrib.PoolID).Msg("Failed to contribute to jackpot pool")
			}
		}
		return nil
	}

	// No default logic - games must implement JackpotHandler to contribute to jackpot pools
	// This ensures games explicitly define their jackpot rules
	s.logger.Debug().Str("game_code", gameCode).Msg("No jackpot contribution - game module does not implement JackpotHandler")
	return nil
}

// processJackpotWin processes a jackpot win
// Uses custom JackpotHandler if implemented by game module, otherwise uses default logic
func (s *GameService) processJackpotWin(
	ctx context.Context,
	spinResult *game.SpinResult,
	userID, username, gameCode, currency string,
	gameConfig *game.Config,
	totalBet decimal.Decimal,
) error {
	if s.rewardProvider == nil {
		return nil
	}

	var jackpotWin *game.JackpotWin
	var err error

	// Check if game module implements custom jackpot handler
	if handler, ok := s.gameModule.(game.JackpotHandler); ok {
		// Use custom jackpot handler
		jackpotWin, err = handler.GetWin(ctx, spinResult, totalBet)
		if err != nil {
			return fmt.Errorf("failed to get jackpot win: %w", err)
		}
		if jackpotWin == nil {
			// No jackpot win detected by custom handler
			return nil
		}
	} else {
		// No default logic - games must implement JackpotHandler to process jackpot wins
		s.logger.Warn().Str("game_code", gameCode).Msg("Jackpot win detected but game module does not implement JackpotHandler - skipping jackpot processing")
		return nil
	}

	// Claim jackpot
	claim, err := s.rewardProvider.Claim(ctx, &providers.ClaimRequest{
		PoolID:    jackpotWin.PoolID,
		UserID:    userID,
		GameCode:  gameCode,
		InitValue: jackpotWin.InitValue,
	})
	if err != nil {
		return err
	}

	// Update spin result with jackpot win
	spinResult.TotalWinJackpot = claim.Amount
	spinResult.JackpotType = &jackpotWin.Tier
	// spinResult.TotalWin = spinResult.TotalWin.Add(claim.Amount)

	// Payout for jackpot win
	if claim.Amount.GreaterThan(decimal.Zero) {
		if s.walletProvider == nil {
			return errors.New(errors.ErrInternalServerError, "wallet provider not configured")
		}
		if err := s.walletProvider.Deposit(ctx, userID, currency, claim.Amount); err != nil {
			return errors.Wrap(err, errors.ErrWalletError, "failed to deposit winnings")
		}
	}

	// Log jackpot win
	// if s.logProvider != nil {
	// 	_, err := s.logProvider.LogJackpot(ctx, &JackpotLog{
	// 		UserID:    userID,
	// 		Username:  username,
	// 		GameCode:  gameCode,
	// 		Tier:      jackpotWin.Tier,
	// 		BetAmount: totalBet.InexactFloat64(),
	// 		WinAmount: claim.Amount.InexactFloat64(),
	// 		Currency:  currency,
	// 		Timestamp: time.Now().UTC(),
	// 	})
	// 	if err != nil {
	// 		s.logger.Error().Err(err).Msg("Failed to log jackpot")
	// 	}
	// }

	return nil
}
