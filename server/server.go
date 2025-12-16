package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/auth"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/config"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/middleware"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/jackpot"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/providers"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// App represents the game service application
type App struct {
	engine            *gin.Engine
	config            *config.Config
	logger            zerolog.Logger
	gameModule        game.Module
	gameServiceFactory GameServiceFactory
	httpServer        *http.Server
	onShutdown        []func()
	gameHandler       *GameHandler
	jackpotHandler    *JackpotHandler
	jackpotService    *jackpot.Service
	jackpotFeedCancel context.CancelFunc
	stateProvider     providers.StateProvider
	walletProvider    providers.WalletProvider
	rewardProvider    providers.RewardProvider
	logProvider       providers.LogProvider
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
			logger zerolog.Logger,
		) SpinService {
			return NewGameService(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, logger)
		},
	}

	// Jackpot service (buffered + broadcast interval)
	app.jackpotService = jackpot.NewService(jackpot.ServiceConfig{
		BroadcastInterval: 2 * time.Second,
		Logger:            opts.Logger,
	})

	// Create handlers
	app.gameHandler = NewGameHandler(app)
	app.jackpotHandler = NewJackpotHandler(app, app.jackpotService)

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
	logger zerolog.Logger,
) SpinService {
	if a.gameServiceFactory != nil {
		return a.gameServiceFactory(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, logger)
	}
	return NewGameService(gameModule, stateProvider, walletProvider, rewardProvider, logProvider, logger)
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

func (a *App) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   a.config.Environment,
		"game_code": a.GetGameCode(),
	})
}

// RegisterCommonGameRoutes registers common game API routes automatically
//
// Flow: HTTP Request -> gameRoutes -> GameHandler -> GameService -> GameModule
//
// Routes registered:
//   - POST /api/games/{game_code}/authorize-game  -> GameHandler.Authorize
//   - POST /api/games/{game_code}/spin            -> GameHandler.Spin -> GameService -> GameModule
//   - GET  /api/games/{game_code}/config          -> GameHandler.GetConfig
//   - GET  /api/games/{game_code}/get-player-state -> GameHandler.GetState
//   - GET  /api/games/{game_code}/jackpot/updates -> JackpotHandler.StreamUpdates (SSE)
//   - GET  /api/games/{game_code}/jackpot/updates/ws -> JackpotHandler.StreamUpdatesWebSocket (WebSocket)
//   - GET  /api/games/{game_code}/bet-history    -> GameHandler.GetBetHistory
//
// For custom routes, use CustomRoutes() to add game-specific endpoints.
func (a *App) RegisterCommonGameRoutes() {
	if a.gameModule == nil {
		a.logger.Fatal().Msg("No game module registered. Call RegisterGame() first.")
		return
	}

	gameCode := a.gameModule.GetGameCode()

	games := a.engine.Group("/api/games")
	games.Use(auth.JWTMiddleware(a.config.JWT.Secret, a.logger)) // JWT middleware sets user info
	games.Use(a.ModuleContextMiddleware())                       // ModuleContext middleware injects context
	{
		gameRoutes := games.Group("/" + gameCode)
		{
			// Core game routes
			gameRoutes.POST("/authorize-game", a.gameHandler.Authorize)
			gameRoutes.POST("/spin", a.gameHandler.Spin)
			gameRoutes.GET("/config", a.gameHandler.GetConfig)
			gameRoutes.GET("/get-player-state", a.gameHandler.GetState)

			// Jackpot routes (SSE and WebSocket streams). Updates should be fed via AttachJackpotUpdateFeed (e.g., Kafka consumer).
			gameRoutes.GET("/jackpot/updates", a.jackpotHandler.StreamUpdates)           // SSE endpoint
			gameRoutes.GET("/jackpot/updates/ws", a.jackpotHandler.StreamUpdatesWebSocket) // WebSocket endpoint

			// History route
			gameRoutes.GET("/bet-history", a.gameHandler.GetBetHistory)
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
