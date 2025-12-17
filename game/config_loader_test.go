package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromDirInto(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create first config file (base config)
	baseConfig := `game_code: test-game
game_name: Test Game
pay_line: 20
reel_rows: 3
reel_cols: 5
reel_size: [3, 3, 3, 3, 3]
bet_multipliers: [1.0, 2.0, 5.0]
wild_symbol: 10
scatter_symbol: 11
jackpot_symbol: 12
rtp: 96.5
volatility: medium
`
	baseFile := filepath.Join(tmpDir, "base.yaml")
	if err := os.WriteFile(baseFile, []byte(baseConfig), 0644); err != nil {
		t.Fatalf("Failed to write base config: %v", err)
	}

	// Create second config file (override some values)
	overrideConfig := `game_name: Overridden Game Name
pay_line: 25
reel_rows: 4
jackpot_multiplier: 5000
`
	overrideFile := filepath.Join(tmpDir, "override.yaml")
	if err := os.WriteFile(overrideFile, []byte(overrideConfig), 0644); err != nil {
		t.Fatalf("Failed to write override config: %v", err)
	}

	// Create third config file (override more values)
	finalConfig := `reel_cols: 6
rtp: 97.0
volatility: high
`
	finalFile := filepath.Join(tmpDir, "final.yaml")
	if err := os.WriteFile(finalFile, []byte(finalConfig), 0644); err != nil {
		t.Fatalf("Failed to write final config: %v", err)
	}

	// Load config from directory
	var cfg Config
	if err := LoadConfigFromDirInto(tmpDir, &cfg); err != nil {
		t.Fatalf("Failed to load config from directory: %v", err)
	}

	// Verify merged values
	// Values from base.yaml
	if cfg.GameCode != "test-game" {
		t.Errorf("Expected GameCode 'test-game', got '%s'", cfg.GameCode)
	}
	if cfg.ReelSize == nil || len(cfg.ReelSize) != 5 {
		t.Errorf("Expected ReelSize [3,3,3,3,3], got %v", cfg.ReelSize)
	}
	if cfg.WildSymbol != 10 {
		t.Errorf("Expected WildSymbol 10, got %d", cfg.WildSymbol)
	}

	// Values overridden by override.yaml
	if cfg.GameName != "Overridden Game Name" {
		t.Errorf("Expected GameName 'Overridden Game Name', got '%s'", cfg.GameName)
	}
	if cfg.PayLine != 25 {
		t.Errorf("Expected PayLine 25, got %d", cfg.PayLine)
	}
	if cfg.ReelRows != 4 {
		t.Errorf("Expected ReelRows 4, got %d", cfg.ReelRows)
	}
	if cfg.JackpotMultiplier != 5000 {
		t.Errorf("Expected JackpotMultiplier 5000, got %d", cfg.JackpotMultiplier)
	}

	// Values overridden by final.yaml
	if cfg.ReelCols != 6 {
		t.Errorf("Expected ReelCols 6, got %d", cfg.ReelCols)
	}
	if cfg.RTP != 97.0 {
		t.Errorf("Expected RTP 97.0, got %f", cfg.RTP)
	}
	if cfg.Volatility != "high" {
		t.Errorf("Expected Volatility 'high', got '%s'", cfg.Volatility)
	}
}

func TestLoadConfigFromDirInto_SingleFile(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create single config file
	singleConfig := `game_code: single-game
game_name: Single File Game
pay_line: 30
reel_rows: 5
reel_cols: 7
`
	singleFile := filepath.Join(tmpDir, "single.yaml")
	if err := os.WriteFile(singleFile, []byte(singleConfig), 0644); err != nil {
		t.Fatalf("Failed to write single config: %v", err)
	}

	// Load config from directory
	var cfg Config
	if err := LoadConfigFromDirInto(tmpDir, &cfg); err != nil {
		t.Fatalf("Failed to load config from directory: %v", err)
	}

	// Verify values
	if cfg.GameCode != "single-game" {
		t.Errorf("Expected GameCode 'single-game', got '%s'", cfg.GameCode)
	}
	if cfg.GameName != "Single File Game" {
		t.Errorf("Expected GameName 'Single File Game', got '%s'", cfg.GameName)
	}
	if cfg.PayLine != 30 {
		t.Errorf("Expected PayLine 30, got %d", cfg.PayLine)
	}
	if cfg.ReelRows != 5 {
		t.Errorf("Expected ReelRows 5, got %d", cfg.ReelRows)
	}
	if cfg.ReelCols != 7 {
		t.Errorf("Expected ReelCols 7, got %d", cfg.ReelCols)
	}
}

func TestLoadConfigFromDirInto_NoYamlFiles(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Try to load from empty directory
	var cfg Config
	err = LoadConfigFromDirInto(tmpDir, &cfg)
	if err == nil {
		t.Error("Expected error when loading from empty directory, got nil")
	}
	if err != nil && err.Error() == "" {
		t.Error("Expected error message, got empty string")
	}
}

func TestLoadConfigFromDirInto_NonExistentDir(t *testing.T) {
	// Try to load from non-existent directory
	var cfg Config
	err := LoadConfigFromDirInto("/non/existent/dir", &cfg)
	if err == nil {
		t.Error("Expected error when loading from non-existent directory, got nil")
	}
}

func TestLoadConfigFromDirInto_AlphabeticalOrder(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files in non-alphabetical order
	// z-file.yaml should be loaded last and override everything
	zConfig := `pay_line: 100
game_name: Z File Game
`
	zFile := filepath.Join(tmpDir, "z-file.yaml")
	if err := os.WriteFile(zFile, []byte(zConfig), 0644); err != nil {
		t.Fatalf("Failed to write z config: %v", err)
	}

	// a-file.yaml should be loaded first
	aConfig := `game_code: test-game
pay_line: 20
game_name: A File Game
`
	aFile := filepath.Join(tmpDir, "a-file.yaml")
	if err := os.WriteFile(aFile, []byte(aConfig), 0644); err != nil {
		t.Fatalf("Failed to write a config: %v", err)
	}

	// m-file.yaml should be loaded in the middle
	mConfig := `pay_line: 50
game_name: M File Game
`
	mFile := filepath.Join(tmpDir, "m-file.yaml")
	if err := os.WriteFile(mFile, []byte(mConfig), 0644); err != nil {
		t.Fatalf("Failed to write m config: %v", err)
	}

	// Load config from directory
	var cfg Config
	if err := LoadConfigFromDirInto(tmpDir, &cfg); err != nil {
		t.Fatalf("Failed to load config from directory: %v", err)
	}

	// Verify that z-file.yaml (loaded last) overrides everything
	if cfg.PayLine != 100 {
		t.Errorf("Expected PayLine 100 (from z-file.yaml, loaded last), got %d", cfg.PayLine)
	}
	if cfg.GameName != "Z File Game" {
		t.Errorf("Expected GameName 'Z File Game' (from z-file.yaml, loaded last), got '%s'", cfg.GameName)
	}
	// GameCode should be from a-file.yaml (first file)
	if cfg.GameCode != "test-game" {
		t.Errorf("Expected GameCode 'test-game' (from a-file.yaml, loaded first), got '%s'", cfg.GameCode)
	}
}

func TestLoadGameConfig_FromDirectory(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create base config
	baseConfig := `game_code: test-game
pay_line: 20
reel_rows: 3
reel_cols: 5
`
	baseFile := filepath.Join(tmpDir, "base.yaml")
	if err := os.WriteFile(baseFile, []byte(baseConfig), 0644); err != nil {
		t.Fatalf("Failed to write base config: %v", err)
	}

	// Create override config
	overrideConfig := `pay_line: 25
reel_rows: 4
`
	overrideFile := filepath.Join(tmpDir, "override.yaml")
	if err := os.WriteFile(overrideFile, []byte(overrideConfig), 0644); err != nil {
		t.Fatalf("Failed to write override config: %v", err)
	}

	// Load using LoadGameConfig
	cfg, err := LoadGameConfig[Config](tmpDir)
	if err != nil {
		t.Fatalf("Failed to load game config: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil config, got nil")
	}

	// Verify merged values
	if cfg.GameCode != "test-game" {
		t.Errorf("Expected GameCode 'test-game', got '%s'", cfg.GameCode)
	}
	if cfg.PayLine != 25 {
		t.Errorf("Expected PayLine 25 (overridden), got %d", cfg.PayLine)
	}
	if cfg.ReelRows != 4 {
		t.Errorf("Expected ReelRows 4 (overridden), got %d", cfg.ReelRows)
	}
	if cfg.ReelCols != 5 {
		t.Errorf("Expected ReelCols 5 (from base), got %d", cfg.ReelCols)
	}
}
