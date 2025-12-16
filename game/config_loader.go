package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// LoadConfig loads game configuration from a YAML file
// This is a helper function to reduce boilerplate in game modules
func LoadConfig(configPath string) (*Config, error) {
	var cfg Config
	if err := LoadConfigInto(configPath, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadConfigInto loads config into the provided struct (out must be a pointer).
func LoadConfigInto(configPath string, out interface{}) error {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if err := v.Unmarshal(out); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// LoadConfigFromDir loads configuration from a directory, merging all YAML files
// Files are loaded in alphabetical order, with later files overriding earlier ones
// This allows splitting config into multiple files (e.g., module-base.yml, game-specific.yaml)
func LoadConfigFromDir(configDir string) (*Config, error) {
	var cfg Config
	if err := LoadConfigFromDirInto(configDir, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadConfigFromDirInto loads config from a directory into the provided struct (out must be a pointer).
// All YAML files in the directory are loaded and merged, with later files (alphabetically) overriding earlier ones.
func LoadConfigFromDirInto(configDir string, out interface{}) error {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read all YAML files in the directory
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return fmt.Errorf("failed to read config directory: %w", err)
	}

	// Sort files alphabetically to ensure consistent loading order
	var yamlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".yaml") || strings.HasSuffix(strings.ToLower(entry.Name()), ".yml") {
			yamlFiles = append(yamlFiles, entry.Name())
		}
	}

	if len(yamlFiles) == 0 {
		return fmt.Errorf("no YAML files found in config directory: %s", configDir)
	}

	// Load each file, with later files overriding earlier ones
	for _, filename := range yamlFiles {
		filePath := filepath.Join(configDir, filename)
		v.SetConfigFile(filePath)
		if err := v.MergeInConfig(); err != nil {
			return fmt.Errorf("failed to merge config from %s: %w", filename, err)
		}
	}

	if err := v.Unmarshal(out); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// LoadGameConfig loads game configuration with custom fields from a file or directory.
// The customConfig struct should embed game.Config to inherit base config fields.
// configPath can be a single YAML file or a directory containing multiple YAML files.
// If directory, all YAML files are merged (e.g., module-base.yml, game-specific.yaml).
//
// Example usage:
//
//	type MyGameConfig struct {
//		Config                    // Embed game.Config to get base config fields
//		JackpotConfig map[string]JackpotTierConfig `mapstructure:"jackpot_config"`
//		CustomField    string                      `mapstructure:"custom_field"`
//	}
//
//	customConfig, err := game.LoadGameConfig[MyGameConfig]("config")
//	if err != nil {
//		return err
//	}
//	// Access base config fields directly: customConfig.PayLine, customConfig.ReelRows, etc.
//	// Access custom fields: customConfig.JackpotConfig, customConfig.CustomField
func LoadGameConfig[T any](configPath string) (*T, error) {
	// Check if configPath is a directory or file
	info, err := os.Stat(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config path: %w", err)
	}

	var customConfig T

	if info.IsDir() {
		// Load from directory (merges all YAML files)
		if err := LoadConfigFromDirInto(configPath, &customConfig); err != nil {
			return nil, fmt.Errorf("failed to load config from directory: %w", err)
		}
	} else {
		// Load from single file
		if err := LoadConfigInto(configPath, &customConfig); err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	return &customConfig, nil
}
