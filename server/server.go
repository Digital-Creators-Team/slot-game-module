package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/auth"
	"github.com/Digital-Creators-Team/slot-game-module/config"
	dbredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/Digital-Creators-Team/slot-game-module/middleware"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/cfsign"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/jackpot"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/providers"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// App represents the game service application
type App struct {
	engine             *gin.Engine
	config             *config.Config
	logger             zerolog.Logger
	gameModule         game.Module
	gameServiceFactory GameServiceFactory
	httpServer         *http.Server
	onShutdown         []func()
	gameHandler        *GameHandler
	jackpotHandler     *JackpotHandler
	eventsWSHandler    *EventsWSHandler
	jackpotService     *jackpot.Service
	jackpotFeedCancel  context.CancelFunc
	stateProvider      providers.StateProvider
	walletProvider     providers.WalletProvider
	rewardProvider     providers.RewardProvider
	logProvider        providers.LogProvider
	tenantProvider     providers.TenantProvider
	gameProvider       providers.GameProvider
	wsConnManager      *WSConnManager
	assetSigner        *cfsign.Signer
}

// Options holds server configuration options
type Options struct {
	Config *config.Config
	Logger zerolog.Logger
}

// GameServiceFactory constructs a GameService implementation.
type GameServiceFactory func(
	gameModule game.Module,
	stateProvider providers.StateProvider,
	walletProvider providers.WalletProvider,
	rewardProvider providers.RewardProvider,
	logProvider providers.LogProvider,
	tenantProvider providers.TenantProvider,
	gameProvider providers.GameProvider,
	logger zerolog.Logger,
) SpinService

// Router is an alias for gin.Engine for convenience
type Router = gin.Engine

// New creates a new game service application
func New(opts Options) *App {
	// Configure decimal.Decimal to marshal as JSON number instead of string
	// WARNING: This may cause precision loss for decimals with many digits when
	// unmarshaled by clients using IEEE 754 double-precision (e.g., JavaScript)
	decimal.MarshalJSONWithoutQuotes = true

	// Set Gin mode
	if opts.Config.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()

	app := &App{
		engine: engine,
		config: opts.Config,
		logger: opts.Logger,
		// Default factory uses the built-in implementation
		gameServiceFactory: func(
			gameModule game.Module,
			stateProvider providers.StateProvider,
			walletProvider providers.WalletProvider,
			rewardProvider providers.RewardProvider,
			logProvider providers.LogProvider,
			tenantProvider providers.TenantProvider,
			gameProvider providers.GameProvider,
			logger zerolog.Logger,
		) SpinService {
			return NewGameService(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, tenantProvider, gameProvider, logger)
		},
	}

	// Jackpot service (buffered + broadcast interval)
	app.jackpotService = jackpot.NewService(jackpot.ServiceConfig{
		Logger: opts.Logger,
	})

	// Create handlers
	app.gameHandler = NewGameHandler(app)
	app.jackpotHandler = NewJackpotHandler(app, app.jackpotService)
	app.wsConnManager = NewWSConnManager(app, opts.Logger)
	redis, _ := dbredis.New(app.config.Redis)
	app.wsConnManager.SetRedisClient(redis)
	app.eventsWSHandler = NewEventsWSHandler(app, app.wsConnManager)
	app.assetSigner = newAssetSigner(opts.Config.CloudFront, opts.Logger)

	if app.gameProvider != nil {
		app.gameProvider.AddDisableGameCallback(context.Background(), app.eventsWSHandler.KickTenantPlayersCallback)
	}

	return app
}

// SetStateProvider sets the state provider for player state management
func (a *App) SetStateProvider(provider StateProvider) {
	a.stateProvider = provider
}

// SetWalletProvider sets the wallet provider for balance operations
func (a *App) SetWalletProvider(provider WalletProvider) {
	a.walletProvider = provider
}

// SetRewardProvider sets the reward provider for jackpot operations
func (a *App) SetRewardProvider(provider RewardProvider) {
	a.rewardProvider = provider
	// Also wire into jackpot service so contributions can be persisted automatically.
	if a.jackpotService != nil {
		a.jackpotService.SetRewardProvider(provider)
	}
}

// SetLogProvider sets the log provider for event logging
func (a *App) SetLogProvider(provider LogProvider) {
	a.logProvider = provider
}

// SetTenantProvider sets the tenant provider for tenant operations
func (a *App) SetTenantProvider(provider TenantProvider) {
	a.tenantProvider = provider
}

// SetGameProvider sets the game provider for game operations
func (a *App) SetGameProvider(provider GameProvider) {
	a.gameProvider = provider

	if a.gameProvider != nil {
		a.gameProvider.AddDisableGameCallback(context.Background(), a.eventsWSHandler.KickTenantPlayersCallback)
	}
}

func (a *App) SetRedisClient(client *dbredis.Client) {
	if a.wsConnManager != nil {
		a.wsConnManager.SetRedisClient(client)
		a.OnShutdown(func() {
			a.wsConnManager.Close()
		})
	}
}

// SetAssetSigner overrides the CloudFront signer (e.g. for tests).
func (a *App) SetAssetSigner(signer *cfsign.Signer) {
	a.assetSigner = signer
}

// AttachJackpotUpdateFeed attaches a source of jackpot updates (e.g., Kafka consumer channel).
// It copies updates into the shared jackpotService buffer. Pass nil to detach.
func (a *App) AttachJackpotUpdateFeed(feed <-chan jackpot.Update) {
	// stop previous feed if any
	if a.jackpotFeedCancel != nil {
		a.jackpotFeedCancel()
		a.jackpotFeedCancel = nil
	}
	if feed == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.jackpotFeedCancel = cancel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case upd, ok := <-feed:
				if !ok {
					return
				}
				a.jackpotService.HandleKafkaUpdate(upd)
			}
		}
	}()
}

// SetGameServiceFactory allows injecting a custom GameService implementation.
// If not set, the default factory is used.
func (a *App) SetGameServiceFactory(factory GameServiceFactory) {
	a.gameServiceFactory = factory
}

// newGameService constructs a GameService using the configured factory.
func (a *App) newGameService(
	gameModule game.Module,
	stateProvider StateProvider,
	walletProvider WalletProvider,
	rewardProvider RewardProvider,
	logProvider LogProvider,
	tenantProvider providers.TenantProvider,
	gameProvider providers.GameProvider,
	logger zerolog.Logger,
) SpinService {
	if a.gameServiceFactory != nil {
		return a.gameServiceFactory(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, tenantProvider, gameProvider, logger)
	}
	return NewGameService(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, tenantProvider, gameProvider, logger)
}

// newAssetSigner builds the CloudFront signer from config.
// Returns nil when CloudFront is not configured; exits if it is configured but invalid.
func newAssetSigner(c config.CloudFrontConfig, logger zerolog.Logger) *cfsign.Signer {
	if c.KeyPairID == "" {
		logger.Warn().Msg("CloudFront is not configured; asset token endpoint is disabled")
		return nil
	}

	pemStr := c.PrivateKey
	if c.PrivateKeyPath != "" {
		b, err := os.ReadFile(c.PrivateKeyPath)
		if err != nil {
			logger.Fatal().Err(err).Str("path", c.PrivateKeyPath).Msg("Failed to read CloudFront private key")
		}
		pemStr = string(b)
	}
	// Accept single-line PEM with literal "\n" sequences.
	pemStr = strings.ReplaceAll(pemStr, `\n`, "\n")

	signer, err := cfsign.NewSigner(c.KeyPairID, pemStr, c.CDNDomain)
	if err != nil {
		logger.Fatal().Err(err).Msg("Invalid CloudFront configuration")
	}

	logger.Info().Str("cdn_domain", c.CDNDomain).Msg("CloudFront asset signer initialized")
	return signer
}

// UseCommonMiddlewares adds common middlewares to the application
func (a *App) UseCommonMiddlewares() {
	// Recovery middleware (must be first)
	a.engine.Use(middleware.Recovery(a.logger))

	// Trace ID middleware
	a.engine.Use(middleware.TraceID())

	// Logging middleware
	a.engine.Use(middleware.Logging(a.logger))

	// CORS middleware if enabled
	if a.config.Server.EnableCORS {
		a.engine.Use(middleware.CORS())
	}
}

// UseMiddleware adds a custom middleware
func (a *App) UseMiddleware(m gin.HandlerFunc) {
	a.engine.Use(m)
}

// RegisterGame registers THE game module for this service
func (a *App) RegisterGame(module game.Module) {
	a.gameModule = module
	a.logger.Info().Str("game_code", module.GetGameCode()).Msg("Game module registered")
	// Keep jackpot service aware of current game code for contribution logging.
	if a.jackpotService != nil {
		a.jackpotService.SetGameCode(module.GetGameCode())
		err := a.bootstrapJackpotPools()
		if err != nil {
			a.logger.Fatal().Err(err).Msg("Failed to bootstrap jackpot pools")
		}
	}
	if a.gameProvider != nil {
		a.gameProvider.SetGameCode(module.GetGameCode())
	}
}

// GetGame returns the registered game module
func (a *App) GetGame() game.Module {
	return a.gameModule
}

// GetJackpotService returns the jackpot service
func (a *App) GetJackpotService() *jackpot.Service {
	return a.jackpotService
}

func (a *App) bootstrapJackpotPools() error {
	handler, ok := a.gameModule.(game.JackpotHandler)
	if !ok || a.jackpotService == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	appConfig, err := a.gameModule.GetConfig(ctx)
	if err != nil {
		return err
	}
	gameCode := a.gameModule.GetGameCode()
	cfg := appConfig.GetConfig()

	for _, t := range cfg.Tier {
		for _, mul := range cfg.Multiplier {
			bet := t * mul
			poolIDs, err := handler.GetPoolID(ctx, cfg.DefaultTenantID,
				cfg.DefaultCurrency, gameCode, bet)
			if err != nil {
				return err
			}
			for _, pid := range poolIDs {
				init, err := handler.GetInitialPoolValue(ctx, pid, bet)
				if err != nil {
					return err
				}
				a.jackpotService.RegisterPool(jackpot.PoolConfig{ID: pid, Init: init})
			}
		}
	}
	return a.jackpotService.InitializePoolsFromProvider(ctx)
}

// GetStateProvider returns the state provider
func (a *App) GetStateProvider() providers.StateProvider {
	return a.stateProvider
}

// GetWalletProvider returns the wallet provider
func (a *App) GetWalletProvider() providers.WalletProvider {
	return a.walletProvider
}

// GetRewardProvider returns the reward provider
func (a *App) GetRewardProvider() providers.RewardProvider {
	return a.rewardProvider
}

// GetLogProvider returns the log provider
func (a *App) GetLogProvider() providers.LogProvider {
	return a.logProvider
}

// GetTenantProvider returns the tenant provider
func (a *App) GetTenantProvider() providers.TenantProvider {
	return a.tenantProvider
}

// GetGameProvider returns the game provider
func (a *App) GetGameProvider() providers.GameProvider {
	return a.gameProvider
}

// GetGameCode returns the game code of registered module
func (a *App) GetGameCode() string {
	if a.gameModule == nil {
		return ""
	}
	return a.gameModule.GetGameCode()
}

// RegisterHealthCheck adds health check endpoints
func (a *App) RegisterHealthCheck() {
	a.engine.GET("/health", a.healthCheck)
	a.engine.GET("/api/health", a.healthCheck)
}

// GetAssetSigner returns the CloudFront signer, or nil if CloudFront is not configured.
func (a *App) GetAssetSigner() *cfsign.Signer {
	return a.assetSigner
}

func (a *App) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   a.config.Environment,
		"game_code": a.GetGameCode(),
		"version":   GetVersion(),
	})
}

// RegisterCommonGameRoutes registers common game API routes automatically
//
// Flow: HTTP Request -> gameRoutes -> GameHandler -> GameService -> GameModule
//
// Routes registered:
//   - GET /api/games/{game_code}/authorize-game  -> GameHandler.Authorize (requires auth)
//   - POST /api/games/{game_code}/spin            -> GameHandler.Spin -> GameService -> GameModule (requires auth)
//   - GET  /api/games/{game_code}/config          -> GameHandler.GetConfig (public, no auth)
//   - GET  /api/games/{game_code}/get-player-state -> GameHandler.GetState (requires auth)
//   - GET  /api/games/{game_code}/jackpot/updates -> JackpotHandler.StreamUpdates (public, no auth)
//   - GET  /api/games/{game_code}/jackpot/updates/ws -> JackpotHandler.StreamUpdatesWebSocket (public, no auth)
//   - GET  /api/games/{game_code}/bet-history    -> GameHandler.GetBetHistory (requires auth)
//
// For custom routes, use CustomRoutes() to add game-specific endpoints.
func (a *App) RegisterCommonGameRoutes() {
	if a.gameModule == nil {
		a.logger.Fatal().Msg("No game module registered. Call RegisterGame() first.")
		return
	}

	gameCode := a.gameModule.GetGameCode()

	// Base group for all game routes (no auth middleware)
	games := a.engine.Group("/api/games")
	{
		gameRoutes := games.Group("/" + gameCode)
		{
			// Public routes (no authentication required)
			gameRoutes.GET("/config", a.gameHandler.GetConfig, a.ModuleContextMiddleware())
			gameRoutes.GET("/jackpot/updates", a.jackpotHandler.StreamUpdates, a.ModuleContextMiddleware())             // SSE endpoint
			gameRoutes.GET("/jackpot/updates/ws", a.jackpotHandler.StreamUpdatesWebSocket, a.ModuleContextMiddleware()) // WebSocket endpoint
			gameRoutes.GET("/events/ws", a.eventsWSHandler.Stream)

			// Protected routes (require JWT authentication)
			authRoutes := gameRoutes.Group("")
			authRoutes.Use(auth.JWTMiddleware(a.config.JWT.Secret, a.logger), // JWT middleware sets user info
				a.ModuleContextMiddleware())
			{
				authRoutes.GET("/authorize-game", a.gameHandler.Authorize)
				authRoutes.POST("/spin", a.gameHandler.Spin)
				authRoutes.GET("/get-player-state", a.gameHandler.GetState)
				authRoutes.GET("/bet-history", a.gameHandler.GetBetHistory)
				authRoutes.GET("/assets-token", a.gameHandler.AssetsToken)
			}
		}
	}

	a.logger.Info().
		Str("game_code", gameCode).
		Msg("Common game routes registered: /api/games/" + gameCode)
}

// CustomRoutes returns a router group for adding custom routes to the game
// Example: app.CustomRoutes().GET("/jackpot-info", handler)
func (a *App) CustomRoutes() *gin.RouterGroup {
	if a.gameModule == nil {
		a.logger.Fatal().Msg("No game module registered. Call RegisterGame() first.")
		return nil
	}

	gameCode := a.gameModule.GetGameCode()
	games := a.engine.Group("/api/games")
	games.Use(auth.JWTMiddleware(a.config.JWT.Secret, a.logger))
	return games.Group("/" + gameCode)
}

// Router returns the Gin engine for custom route registration
func (a *App) Router() *gin.Engine {
	return a.engine
}

// Group creates a route group
func (a *App) Group(path string, handlers ...gin.HandlerFunc) *gin.RouterGroup {
	return a.engine.Group(path, handlers...)
}

// AuthGroup creates a route group with JWT authentication
func (a *App) AuthGroup(path string) *gin.RouterGroup {
	return a.engine.Group(path, auth.JWTMiddleware(a.config.JWT.Secret, a.logger))
}

// RegisterRoutes registers custom routes using a callback
func (a *App) RegisterRoutes(fn func(*gin.Engine)) {
	fn(a.engine)
}

// OnShutdown registers a function to be called on shutdown
func (a *App) OnShutdown(fn func()) {
	a.onShutdown = append(a.onShutdown, fn)
}

// Run starts the HTTP server
func (a *App) Run() error {
	addr := fmt.Sprintf(":%d", a.config.Server.Port)

	a.httpServer = &http.Server{
		Addr:         addr,
		Handler:      a.engine,
		ReadTimeout:  a.config.Server.ReadTimeout,
		WriteTimeout: a.config.Server.WriteTimeout,
		IdleTimeout:  a.config.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		a.logger.Info().
			Int("port", a.config.Server.Port).
			Str("environment", a.config.Environment).
			Str("game_code", a.GetGameCode()).
			Msg("Starting HTTP server")

		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal
	return a.waitForShutdown()
}

// RunWithContext starts the HTTP server with context
func (a *App) RunWithContext(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", a.config.Server.Port)

	a.httpServer = &http.Server{
		Addr:         addr,
		Handler:      a.engine,
		ReadTimeout:  a.config.Server.ReadTimeout,
		WriteTimeout: a.config.Server.WriteTimeout,
		IdleTimeout:  a.config.Server.IdleTimeout,
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		a.logger.Info().
			Int("port", a.config.Server.Port).
			Str("environment", a.config.Environment).
			Str("game_code", a.GetGameCode()).
			Msg("Starting HTTP server")

		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		return a.shutdown()
	case err := <-errChan:
		return err
	}
}

func (a *App) waitForShutdown() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	return a.shutdown()
}

func (a *App) shutdown() error {
	a.logger.Info().Msg("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call registered shutdown handlers
	for _, fn := range a.onShutdown {
		fn()
	}

	// Shutdown lifecycle hooks for game
	if a.gameModule != nil {
		if lh, ok := a.gameModule.(game.LifecycleHooks); ok {
			if err := lh.OnShutdown(ctx); err != nil {
				a.logger.Error().Err(err).Msg("Error during game shutdown")
			}
		}
	}

	// Shutdown HTTP server
	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.logger.Error().Err(err).Msg("Error during server shutdown")
		return err
	}

	a.logger.Info().Msg("Server shutdown complete")
	return nil
}

// Config returns the application configuration
func (a *App) Config() *config.Config {
	return a.config
}

// Logger returns the application logger
func (a *App) Logger() zerolog.Logger {
	return a.logger
}

// GameHandler returns the built-in game handler
func (a *App) GameHandler() *GameHandler {
	return a.gameHandler
}
