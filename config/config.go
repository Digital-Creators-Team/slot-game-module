package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	Environment      string                 `mapstructure:"environment"`
	Server           ServerConfig           `mapstructure:"server"`
	Redis            RedisConfig            `mapstructure:"redis"`
	Kafka            KafkaConfig            `mapstructure:"kafka"`
	JWT              JWTConfig              `mapstructure:"jwt"`
	Logging          logging.Config         `mapstructure:"logging"`
	ExternalServices ExternalServicesConfig `mapstructure:"external_services"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	EnableCORS   bool          `mapstructure:"enable_cors"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr         string `mapstructure:"addr"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers       []string          `mapstructure:"brokers"`
	ConsumerGroup string            `mapstructure:"consumer_group"`
	Topics        map[string]string `mapstructure:"topics"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Expiration time.Duration `mapstructure:"expiration"`
}

// ExternalServicesConfig holds external service configurations
type ExternalServicesConfig struct {
	WalletService ServiceConfig `mapstructure:"wallet_service"`
	RewardService ServiceConfig `mapstructure:"reward_service"`
	LogService    ServiceConfig `mapstructure:"log_service"`
}

// ServiceConfig holds external service configuration
type ServiceConfig struct {
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

// Load loads configuration from YAML file using Viper
func Load(filename string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(filename)
	v.SetConfigType("yaml")

	// Enable environment variable substitution
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set defaults
	config.setDefaults()

	return &config, nil
}

// LoadByEnv loads configuration based on environment using Viper
func LoadByEnv(configDir string) (*Config, error) {
	v := viper.New()

	// Set config search paths
	v.AddConfigPath(configDir)
	v.AddConfigPath(".")

	// Get environment
	env := viper.GetString("ENV")
	if env == "" {
		env = viper.GetString("APP_ENV")
	}
	if env == "" {
		env = "development"
	}

	v.SetConfigName(fmt.Sprintf("config-%s", env))
	v.SetConfigType("yaml")

	// Enable environment variable substitution
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	config.setDefaults()

	return &config, nil
}

// LoadWithViper loads configuration and returns the viper instance for custom usage
func LoadWithViper(filename string) (*Config, *viper.Viper, error) {
	v := viper.New()

	v.SetConfigFile(filename)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	config.setDefaults()

	return &config, v, nil
}

// setDefaults sets default values for missing configuration
func (c *Config) setDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = 60 * time.Second
	}
	if c.Redis.PoolSize == 0 {
		c.Redis.PoolSize = 10
	}
	if c.Redis.MinIdleConns == 0 {
		c.Redis.MinIdleConns = 5
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = "stdout"
	}
	// Service defaults
	if c.ExternalServices.WalletService.Timeout == 0 {
		c.ExternalServices.WalletService.Timeout = 10 * time.Second
	}
	if c.ExternalServices.RewardService.Timeout == 0 {
		c.ExternalServices.RewardService.Timeout = 10 * time.Second
	}
	if c.ExternalServices.LogService.Timeout == 0 {
		c.ExternalServices.LogService.Timeout = 10 * time.Second
	}
}

// GetAddr returns Redis address
func (c *RedisConfig) GetAddr() string {
	return c.Addr
}

// IsDevelopment returns true if environment is development
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development" || c.Environment == "dev"
}

// IsProduction returns true if environment is production
func (c *Config) IsProduction() bool {
	return c.Environment == "production" || c.Environment == "prod"
}
