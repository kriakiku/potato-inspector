package config

import (
	"testing"
)

func TestFromEnvProfileDefaults(t *testing.T) {
	t.Setenv("POTATONETWORK_API_TOKEN", "")
	t.Setenv("POTATONETWORK_API_TOKEN_FILE", "")
	t.Setenv("POTATONETWORK_PROFILE_COUNTRY", "bd")
	t.Setenv("POTATONETWORK_PROFILE_TIER", "")
	cfg := FromEnv()
	if cfg.ProfileCountry != "BD" {
		t.Fatalf("country=%q", cfg.ProfileCountry)
	}
	if cfg.ProfileTier != "typical" {
		t.Fatalf("tier=%q", cfg.ProfileTier)
	}
}

func TestFromEnvPassthroughWhenNoCountry(t *testing.T) {
	t.Setenv("POTATONETWORK_PROFILE_COUNTRY", "")
	t.Setenv("POTATONETWORK_PROFILE_TIER", "poor")
	cfg := FromEnv()
	if cfg.ProfileCountry != "" || cfg.ProfileTier != "" {
		t.Fatalf("expected empty boot profile, got %s/%s", cfg.ProfileCountry, cfg.ProfileTier)
	}
}
