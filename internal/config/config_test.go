package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/andycarlberg/audiomoments/internal/config"
)

func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"SUBSONIC_URL",
		"SUBSONIC_USER",
		"SUBSONIC_PASSWORD",
		"STATION_BANK_PATH",
		"TIMEZONE",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

func TestLoad_FromEnv_Valid(t *testing.T) {
	clearEnv(t)
	t.Setenv("SUBSONIC_URL", "  http://localhost:4533/  ")
	t.Setenv("SUBSONIC_USER", "  admin  ")
	t.Setenv("SUBSONIC_PASSWORD", "secret123")
	t.Setenv("STATION_BANK_PATH", "  /custom/stations  ")
	t.Setenv("TIMEZONE", "  America/Chicago  ")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SubsonicURL != "http://localhost:4533" {
		t.Errorf("expected SubsonicURL 'http://localhost:4533', got %q", cfg.SubsonicURL)
	}
	if cfg.SubsonicUser != "admin" {
		t.Errorf("expected SubsonicUser 'admin', got %q", cfg.SubsonicUser)
	}
	if cfg.SubsonicPassword != "secret123" {
		t.Errorf("expected SubsonicPassword 'secret123', got %q", cfg.SubsonicPassword)
	}
	if cfg.StationBankPath != "/custom/stations" {
		t.Errorf("expected StationBankPath '/custom/stations', got %q", cfg.StationBankPath)
	}
	if cfg.Timezone != "America/Chicago" {
		t.Errorf("expected Timezone 'America/Chicago', got %q", cfg.Timezone)
	}
	if cfg.Location == nil || cfg.Location.String() != "America/Chicago" {
		t.Errorf("expected Location 'America/Chicago', got %v", cfg.Location)
	}
}

func TestLoad_Defaults(t *testing.T) {
	tests := []struct {
		name            string
		stationBankPath string
		timezone        string
	}{
		{
			name:            "unset optional fields",
			stationBankPath: "",
			timezone:        "",
		},
		{
			name:            "whitespace optional fields",
			stationBankPath: "   ",
			timezone:        "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("SUBSONIC_URL", "https://music.example.com")
			t.Setenv("SUBSONIC_USER", "user")
			t.Setenv("SUBSONIC_PASSWORD", "pass")
			t.Setenv("STATION_BANK_PATH", tt.stationBankPath)
			t.Setenv("TIMEZONE", tt.timezone)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.StationBankPath != "./stations" {
				t.Errorf("expected default StationBankPath './stations', got %q", cfg.StationBankPath)
			}
			if cfg.Location != time.Local {
				t.Errorf("expected Location %v, got %v", time.Local, cfg.Location)
			}
			if cfg.Timezone != time.Local.String() {
				t.Errorf("expected Timezone default %q, got %q", time.Local.String(), cfg.Timezone)
			}
		})
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "missing URL",
			env: map[string]string{
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "SUBSONIC_URL is required",
		},
		{
			name: "whitespace-only URL",
			env: map[string]string{
				"SUBSONIC_URL":      "   ",
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "SUBSONIC_URL is required",
		},
		{
			name: "URL missing scheme",
			env: map[string]string{
				"SUBSONIC_URL":      "localhost:4533",
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "invalid SUBSONIC_URL",
		},
		{
			name: "URL unsupported scheme",
			env: map[string]string{
				"SUBSONIC_URL":      "ftp://localhost:4533",
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "invalid SUBSONIC_URL",
		},
		{
			name: "URL missing host",
			env: map[string]string{
				"SUBSONIC_URL":      "http://",
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "invalid SUBSONIC_URL",
		},
		{
			name: "missing User",
			env: map[string]string{
				"SUBSONIC_URL":      "http://localhost:4533",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "SUBSONIC_USER is required",
		},
		{
			name: "whitespace-only User",
			env: map[string]string{
				"SUBSONIC_URL":      "http://localhost:4533",
				"SUBSONIC_USER":     "   ",
				"SUBSONIC_PASSWORD": "pass",
			},
			wantErr: "SUBSONIC_USER is required",
		},
		{
			name: "missing Password",
			env: map[string]string{
				"SUBSONIC_URL":  "http://localhost:4533",
				"SUBSONIC_USER": "admin",
			},
			wantErr: "SUBSONIC_PASSWORD is required",
		},
		{
			name: "invalid Timezone",
			env: map[string]string{
				"SUBSONIC_URL":      "http://localhost:4533",
				"SUBSONIC_USER":     "admin",
				"SUBSONIC_PASSWORD": "pass",
				"TIMEZONE":          "Invalid/NonExistent_Zone",
			},
			wantErr: "invalid TIMEZONE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
