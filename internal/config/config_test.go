package config_test

import (
	"goledger/internal/config"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "9090")
	t.Setenv("DB_PATH", "custom.db")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Host != "0.0.0.0" || cfg.Port != 9090 || cfg.DBPath != "custom.db" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		wantErr bool
	}{
		{"Invalid Port String", "PORT", "invalid", true},
		{"Port Too Low", "PORT", "0", true},
		{"Port Too High", "PORT", "70000", true},
		{"Host With Spaces", "HOST", "invalid host", true},
		{"DB Path With Traversal", "DB_PATH", "../secrets.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOST", "")
			t.Setenv("PORT", "")
			t.Setenv("DB_PATH", "")

			t.Setenv(tt.envKey, tt.envVal)
			_, err := config.Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
