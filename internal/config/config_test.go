package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoad_SwitchesAcrossURLInTestnetWhenUnspecified(t *testing.T) {
	viper.Reset()
	t.Setenv("NETWORK", "testnet")
	t.Setenv("ACROSS_API_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.AcrossAPIURL != "https://testnet.across.to/api" {
		t.Fatalf("AcrossAPIURL = %q, want testnet endpoint", cfg.AcrossAPIURL)
	}
}

func TestLoad_PreservesExplicitAcrossURLInTestnet(t *testing.T) {
	viper.Reset()
	t.Setenv("NETWORK", "testnet")
	t.Setenv("ACROSS_API_URL", "https://custom-across.example/api")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.AcrossAPIURL != "https://custom-across.example/api" {
		t.Fatalf("AcrossAPIURL = %q, want explicit override", cfg.AcrossAPIURL)
	}
}
