package game

import (
	"testing"
)

func TestGetConfigFromNormalizer(t *testing.T) {
	tests := []struct {
		name      string
		normalizer ConfigNormalizer
		wantErr   bool
		validate  func(t *testing.T, cfg *Config)
	}{
		{
			name: "direct *Config",
			normalizer: &Config{
				GameCode: "test-game",
				PayLine:  20,
				ReelRows: 3,
				ReelCols: 5,
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.GameCode != "test-game" {
					t.Errorf("expected GameCode 'test-game', got '%s'", cfg.GameCode)
				}
				if cfg.PayLine != 20 {
					t.Errorf("expected PayLine 20, got %d", cfg.PayLine)
				}
				if cfg.ReelRows != 3 {
					t.Errorf("expected ReelRows 3, got %d", cfg.ReelRows)
				}
				if cfg.ReelCols != 5 {
					t.Errorf("expected ReelCols 5, got %d", cfg.ReelCols)
				}
			},
		},
		{
			name: "custom struct embed Config",
			normalizer: &CustomGameConfig{
				Config: Config{
					GameCode: "custom-game",
					PayLine:  25,
					ReelRows: 4,
					ReelCols: 6,
				},
				CustomField: "custom-value",
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.GameCode != "custom-game" {
					t.Errorf("expected GameCode 'custom-game', got '%s'", cfg.GameCode)
				}
				if cfg.PayLine != 25 {
					t.Errorf("expected PayLine 25, got %d", cfg.PayLine)
				}
				if cfg.ReelRows != 4 {
					t.Errorf("expected ReelRows 4, got %d", cfg.ReelRows)
				}
				if cfg.ReelCols != 6 {
					t.Errorf("expected ReelCols 6, got %d", cfg.ReelCols)
				}
			},
		},
		{
			name: "custom struct with named Config field",
			normalizer: &CustomGameConfigNamed{
				GameConfig: Config{
					GameCode: "named-game",
					PayLine:  30,
					ReelRows: 5,
					ReelCols: 7,
				},
				CustomField: "named-value",
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.GameCode != "named-game" {
					t.Errorf("expected GameCode 'named-game', got '%s'", cfg.GameCode)
				}
				if cfg.PayLine != 30 {
					t.Errorf("expected PayLine 30, got %d", cfg.PayLine)
				}
				if cfg.ReelRows != 5 {
					t.Errorf("expected ReelRows 5, got %d", cfg.ReelRows)
				}
				if cfg.ReelCols != 7 {
					t.Errorf("expected ReelCols 7, got %d", cfg.ReelCols)
				}
			},
		},
		{
			name: "invalid type - no Config field",
			normalizer: &InvalidConfig{
				SomeField: "test",
			},
			wantErr: true,
			validate: nil,
		},
		{
			name: "nil normalizer",
			normalizer: nil,
			wantErr: true,
			validate: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := GetConfigFromNormalizer(tt.normalizer)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				if cfg != nil {
					t.Errorf("expected nil config on error, got %v", cfg)
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if cfg == nil {
				t.Errorf("expected non-nil config, got nil")
				return
			}
			
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

// CustomGameConfig is a test struct that embeds Config
type CustomGameConfig struct {
	Config      // Embedded Config
	CustomField string
}

// Normalize implements ConfigNormalizer
func (c *CustomGameConfig) Normalize() map[string]interface{} {
	return c.Config.Normalize()
}

// CustomGameConfigNamed is a test struct with named Config field
type CustomGameConfigNamed struct {
	GameConfig  Config // Named Config field
	CustomField string
}

// Normalize implements ConfigNormalizer
func (c *CustomGameConfigNamed) Normalize() map[string]interface{} {
	return c.GameConfig.Normalize()
}

// InvalidConfig is a test struct without Config field
type InvalidConfig struct {
	SomeField string
}

// Normalize implements ConfigNormalizer
func (c *InvalidConfig) Normalize() map[string]interface{} {
	return map[string]interface{}{
		"someField": c.SomeField,
	}
}
