package server

import (
	"git.futuregamestudio.net/be-shared/slot-game-module.git/auth"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/errors"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// GameHandler handles common HTTP requests for the game
//
// Flow: HTTP Request -> gameRoutes -> GameHandler -> GameService -> GameModule
//
// Responsibilities:
// - Extract user info from JWT token
// - Validate request parameters
// - Call GameService for business logic
// - Format and return HTTP responses
//
// Game-specific logic should be in GameModule, not here.
type GameHandler struct {
	app    *App
	logger zerolog.Logger
}

// NewGameHandler creates a new game handler
func NewGameHandler(app *App) *GameHandler {
	return &GameHandler{
		app:    app,
		logger: app.logger.With().Str("handler", "game").Logger(),
	}
}

// extractUserID extracts user ID from gin context
func (h *GameHandler) extractUserID(c *gin.Context) (string, error) {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return "", errors.New(errors.ErrUnauthorized, "user_id not found in context")
	}
	return userID, nil
}

// extractCurrencyID extracts currency ID from gin context
func (h *GameHandler) extractCurrencyID(c *gin.Context) string {
	currencyID, ok := auth.GetCurrencyID(c)
	if !ok {
		return "gold"
	}
	return currencyID
}

// Authorize godoc
// @Summary      Authorize game session
// @Description  Validates JWT token, retrieves player state, balance, and game config
// @Tags         game
// @Accept       json
// @Produce      json
// @Param        game_code   path      string  true  "Game code"
// @Success      200  {object}  BaseResponse{data=game.AuthorizeResponse}
// @Failure      401  {object}  BaseResponse
// @Failure      500  {object}  BaseResponse
// @Security     BearerAuth
// @Router       /games/{game_code}/authorize-game [post]
func (h *GameHandler) Authorize(c *gin.Context) {
	ctx := c.Request.Context()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		h.logger.Error().Msg("No game module registered")
		InternalError(c, errors.New(errors.ErrGameModuleNotFound, "Game not configured"))
		return
	}

	// Extract user ID from JWT token
	userID, err := h.extractUserID(c)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to extract user ID")
		Unauthorized(c, errors.New(errors.ErrUnauthorized, "Invalid or missing authentication token"))
		return
	}

	// Extract username and currency
	username, _ := auth.GetUsername(c)
	currencyID := h.extractCurrencyID(c)

	// Get game config
	cfg, err := gameModule.GetConfig(ctx)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get game config")
		InternalError(c, errors.New(errors.ErrInternalServerError, "Failed to retrieve game configuration"))
		return
	}
	gameConfig, ok := cfg.(*game.Config)
	if !ok {
		h.logger.Error().Msg("Invalid game config type")
		InternalError(c, errors.New(errors.ErrInternalServerError, "Invalid game config"))
		return
	}

	// Get player state - requires StateProvider
	playerState, err := h.app.stateProvider.GetPlayerState(ctx, userID, gameModule.GetGameCode())
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get player state")
		InternalError(c, errors.Wrap(err, errors.ErrPlayerStateError, "Failed to get player state"))
		return
	}

		// Get balance - requires WalletProvider
	balance, err := h.app.walletProvider.GetBalance(ctx, userID, currencyID)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get balance")
		InternalError(c, errors.Wrap(err, errors.ErrWalletError, "Failed to get balance"))
		return
	}

	// Build response
	response := game.AuthorizeResponse{
		LastState:  playerState,
		GameConfig: gameConfig,
		Player: game.Player{
			UserID:   userID,
			Username: username,
			Balance:  balance,
			Currency: currencyID,
		},
	}

	OK(c, response)
}

// SpinRequest represents the spin request body
// @Description Spin request payload
type SpinRequest struct {
	// Bet multiplier (required, must be > 0)
	BetMultiplier float32 `json:"betMultiplier" binding:"required" example:"1.0"`
	// Optional cheat payout for testing
	CheatPayout *game.CheatPayout `json:"cheatPayout,omitempty"`
}

// Spin godoc
// @Summary      Execute a spin
// @Description  Executes a slot spin with the given bet multiplier. Handles normal and free spins, wallet transactions, jackpot contributions, and logging.
// @Tags         game
// @Accept       json
// @Produce      json
// @Param        game_code   path      string       true  "Game code"
// @Param        request     body      SpinRequest  true  "Spin request"
// @Success      200      {object}  BaseResponse{data=game.SpinResponse}
// @Failure      400      {object}  BaseResponse
// @Failure      401      {object}  BaseResponse
// @Failure      500      {object}  BaseResponse
// @Security     BearerAuth
// @Router       /games/{game_code}/spin [post]
//
// Flow:
// 1. Extract user info from JWT (userID, username, currencyID)
// 2. Validate request (betMultiplier > 0)
// 3. Get providers (StateProvider, WalletProvider, RewardProvider, LogProvider)
// 4. Create GameService with providers
// 5. Call GameService.ExecuteSpin() -> GameModule.PlayNormalSpin() or PlayFreeSpin()
// 6. Format response and return
func (h *GameHandler) Spin(c *gin.Context) {
	ctx := c.Request.Context()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		h.logger.Error().Msg("No game module registered")
		InternalError(c, errors.New(errors.ErrGameModuleNotFound, "Game not configured"))
		return
	}

	// Extract user ID from JWT token
	userID, err := h.extractUserID(c)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to extract user ID")
		Unauthorized(c, errors.New(errors.ErrUnauthorized, "Invalid or missing authentication token"))
		return
	}

	// Parse request body
	var req SpinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error().Err(err).Msg("Failed to parse spin request")
		BadRequest(c, errors.New(errors.ErrInvalidRequest, "Invalid request payload"))
		return
	}

	// Validate bet multiplier
	if req.BetMultiplier <= 0 {
		BadRequest(c, errors.New(errors.ErrInvalidRequest, "Invalid betMultiplier: must be greater than 0"))
		return
	}

	currencyID := h.extractCurrencyID(c)

	// Create game service with providers
	gameService := h.app.newGameService(
		gameModule,
		h.app.stateProvider,
		h.app.walletProvider,
		h.app.rewardProvider,
		h.app.logProvider,
		h.logger,
	)

	// Get username from JWT
	username, _ := auth.GetUsername(c)

	// Execute spin with full flow
	result, err := gameService.ExecuteSpin(ctx, &SpinServiceRequest{
		UserID:        userID,
		Username:      username,
		CurrencyID:    currencyID,
		BetMultiplier: req.BetMultiplier,
		CheatPayout:   req.CheatPayout,
	})
	if err != nil {
		h.logger.Error().Err(err).
			Str("user_id", userID).
			Float32("bet_multiplier", req.BetMultiplier).
			Msg("Failed to execute spin")
		HandleAppError(c, err)
		return
	}

	// Convert to response format
	response := result.SpinResult.ToSpinResponse()

	h.logger.Info().
		Str("user_id", userID).
		Str("game_code", gameModule.GetGameCode()).
		Str("session_id", result.SessionID).
		Float64("total_bet", result.SpinResult.TotalBet.InexactFloat64()).
		Float64("total_win", result.SpinResult.TotalWin.InexactFloat64()).
		Int("spin_type", result.SpinResult.SpinType).
		Msg("Spin executed")

	OK(c, response)
}

// GameConfigResponse represents the game configuration response
// @Description Game configuration
type GameConfigResponse struct {
	PayLine           int   `json:"payLine" example:"20"`
	JackpotMultiplier int   `json:"jackpotMultiplier" example:"5000"`
	ReelSize          []int `json:"reelSize" example:"3,3,3,3,3"`
}

// GetConfig godoc
// @Summary      Get game configuration
// @Description  Returns the game configuration (paylines, jackpot multiplier, reel size)
// @Tags         game
// @Accept       json
// @Produce      json
// @Param        game_code   path      string  true  "Game code"
// @Success      200  {object}  BaseResponse{data=GameConfigResponse}
// @Failure      500  {object}  BaseResponse
// @Security     BearerAuth
// @Router       /games/{game_code}/config [get]
func (h *GameHandler) GetConfig(c *gin.Context) {
	ctx := c.Request.Context()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		h.logger.Error().Msg("No game module registered")
		InternalError(c, errors.New(errors.ErrGameModuleNotFound, "Game not configured"))
		return
	}

	cfg, err := gameModule.GetConfig(ctx)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get game config")
		InternalError(c, errors.New(errors.ErrInternalServerError, "Failed to retrieve game configuration"))
		return
	}
	normalizer, ok := cfg.(interface{ Normalize() map[string]interface{} })
	if !ok {
		h.logger.Error().Msg("Invalid game config type")
		InternalError(c, errors.New(errors.ErrInternalServerError, "Invalid game config"))
		return
	}

	response := normalizer.Normalize()

	OK(c, response)
}

// GetState godoc
// @Summary      Get player state
// @Description  Returns the current player state for the game
// @Tags         game
// @Accept       json
// @Produce      json
// @Success      200  {object}  BaseResponse{data=game.PlayerState}
// @Failure      401  {object}  BaseResponse
// @Failure      500  {object}  BaseResponse
// @Security     BearerAuth
// @Router       /games/{game_code}/get-player-state [get]
func (h *GameHandler) GetState(c *gin.Context) {
	ctx := c.Request.Context()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		h.logger.Error().Msg("No game module registered")
		InternalError(c, errors.New(errors.ErrGameModuleNotFound, "Game not configured"))
		return
	}

	userID, err := h.extractUserID(c)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to extract user ID")
		Unauthorized(c, errors.New(errors.ErrUnauthorized, "Invalid or missing authentication token"))
		return
	}

	// Get player state - requires StateProvider
	playerState, err := h.app.stateProvider.GetPlayerState(ctx, userID, gameModule.GetGameCode())
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get player state")
		InternalError(c, errors.Wrap(err, errors.ErrPlayerStateError, "Failed to get player state"))
		return
	}

	OK(c, playerState)
}

// BetHistoryQueryParams represents query parameters for bet history
type BetHistoryQueryParams struct {
	Type     BetType `form:"type" binding:"required"`
	GameCode string  `form:"gameCode" binding:"required"`
	Limit    int     `form:"limit" binding:"required"`
	Page     int     `form:"page" binding:"required"`
}

// GetBetHistory godoc
// @Summary      Get bet history
// @Description  Returns the bet history for the current user (normal, free_spin, or jackpot)
// @Tags         history
// @Accept       json
// @Produce      json
// @Param        game_code   path      string  true  "Game code"
// @Param        type       query     string  true   "Bet type (normal, free_spin, jackpot)"
// @Param        gameCode   query     string  true   "Game code"
// @Param        limit      query     int     true   "Items per page"
// @Param        page       query     int     true   "Page number (0-based)"
// @Success      200        {object}  BaseResponse{data=BetHistoryResponse}
// @Failure      400        {object}  BaseResponse
// @Failure      401        {object}  BaseResponse
// @Failure      500        {object}  BaseResponse
// @Security     BearerAuth
// @Router       /games/{game_code}/bet-history [get]
func (h *GameHandler) GetBetHistory(c *gin.Context) {
	ctx := c.Request.Context()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		h.logger.Error().Msg("No game module registered")
		InternalError(c, errors.New(errors.ErrGameModuleNotFound, "Game not configured"))
		return
	}

	// Parse query parameters
	var params BetHistoryQueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		BadRequest(c, errors.New(errors.ErrInvalidRequest, err.Error()))
		return
	}

	// For jackpot history, we don't need userID (show all users)
	var userID string
	if params.Type != BetTypeJackpot {
		var err error
		userID, err = h.extractUserID(c)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to extract user ID")
			Unauthorized(c, errors.New(errors.ErrUnauthorized, "Invalid or missing authentication token"))
			return
		}
	}

	// Validate limits
	if params.Limit <= 0 || params.Limit > 100 {
		params.Limit = 20
	}
	if params.Page < 0 {
		params.Page = 0
	}

	// Build query
	query := &BetHistoryQuery{
		UserID:   userID,
		GameCode: params.GameCode,
		Type:     params.Type,
		Page:     params.Page,
		Limit:    params.Limit,
	}

	// Get history
	result, err := h.app.logProvider.GetBetHistory(ctx, query)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get bet history")
		InternalError(c, errors.Wrap(err, errors.ErrInternalServerError, "Failed to get bet history"))
		return
	}

	OK(c, result)
}
