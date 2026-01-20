// Package testutil provides shared test utilities for tfccli tests.
package testutil

import (
	"testing"

	"github.com/richclement/tfccli/internal/config"
)

// TempHome creates a temporary home directory and returns the path.
// Settings can be optionally pre-populated. The directory is automatically
// cleaned up when the test completes.
func TempHome(t *testing.T, settings *config.Settings) string {
	t.Helper()
	tmpDir := t.TempDir()

	if settings != nil {
		if err := config.Save(settings, tmpDir); err != nil {
			t.Fatalf("testutil.TempHome: failed to save settings: %v", err)
		}
	}

	return tmpDir
}

// DefaultTestSettings returns a minimal valid settings struct for tests.
// Address defaults to "app.terraform.io", DefaultOrg to "test-org".
func DefaultTestSettings() *config.Settings {
	return &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    "app.terraform.io",
				DefaultOrg: "test-org",
				LogLevel:   "info",
			},
		},
	}
}

// MultiContextSettings returns settings with multiple contexts for testing
// context switching.
func MultiContextSettings() *config.Settings {
	return &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    "app.terraform.io",
				DefaultOrg: "default-org",
				LogLevel:   "info",
			},
			"prod": {
				Address:    "tfe.example.com",
				DefaultOrg: "prod-org",
				LogLevel:   "info",
			},
		},
	}
}
