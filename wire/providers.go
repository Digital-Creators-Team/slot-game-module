package wire

import (
	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/server"
	"github.com/google/wire"
	"github.com/rs/zerolog"
)

// ProvideLogger provides a zerolog.Logger
func ProvideLogger(cfg *config.Config) zerolog.Logger {
	return logging.New(cfg.Logging)
}

// ProvideRedisClient provides a Redis client
func ProvideRedisClient(cfg *config.Config) (*redis.Client, error) {
	return redis.New(cfg.Redis)
}

// ProvideServerOptions provides server options
func ProvideServerOptions(cfg *config.Config, logger zerolog.Logger) server.Options {
	return server.Options{
		Config: cfg,
		Logger: logger,
	}
}

// ProvideApp provides the main application
func ProvideApp(opts server.Options) *server.App {
	return server.New(opts)
}

// ConfigSet is the wire provider set for configuration
var ConfigSet = wire.NewSet(
	config.Load,
)

// LoggingSet is the wire provider set for logging
var LoggingSet = wire.NewSet(
	ProvideLogger,
)

// RedisSet is the wire provider set for Redis
var RedisSet = wire.NewSet(
	ProvideRedisClient,
)

// ServerSet is the wire provider set for server
var ServerSet = wire.NewSet(
	ProvideServerOptions,
	ProvideApp,
)

// DefaultSet is the default wire provider set including all common providers
var DefaultSet = wire.NewSet(
	LoggingSet,
	ServerSet,
)

// FullSet includes all providers including Redis
var FullSet = wire.NewSet(
	DefaultSet,
	RedisSet,
)
