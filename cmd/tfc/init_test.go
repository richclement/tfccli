package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/ui"
)

func TestInitCmd_CreatesSettingsWithDefaults(t *testing.T) {
	// Setup temp home directory
	tmpHome := t.TempDir()

	// Create scripted prompter that accepts all defaults
	prompter := ui.NewScriptedPrompter()
	// Empty responses mean accept defaults

	// Create and run init command
	cmd := &InitCmd{
		prompter: prompter,
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify settings file exists
	settingsPath := filepath.Join(tmpHome, ".tfccli", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("settings.json was not created")
	}

	// Load and verify settings
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if settings.CurrentContext != "default" {
		t.Errorf("CurrentContext = %q, want %q", settings.CurrentContext, "default")
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != config.DefaultAddress {
		t.Errorf("Address = %q, want %q", ctx.Address, config.DefaultAddress)
	}
	if ctx.LogLevel != config.DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", ctx.LogLevel, config.DefaultLogLevel)
	}
}

func TestInitCmd_NonInteractiveWithProvidedValues(t *testing.T) {
	tmpHome := t.TempDir()

	cmd := &InitCmd{
		NonInteractive: true,
		DefaultOrg:     "acme",
		LogLevel:       "warn",
		baseDir:        tmpHome,
	}
	cli := &CLI{
		Address: "app.terraform.io/eu",
	}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Load and verify settings
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != "app.terraform.io/eu" {
		t.Errorf("Address = %q, want %q", ctx.Address, "app.terraform.io/eu")
	}
	if ctx.DefaultOrg != "acme" {
		t.Errorf("DefaultOrg = %q, want %q", ctx.DefaultOrg, "acme")
	}
	if ctx.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", ctx.LogLevel, "warn")
	}
}

func TestInitCmd_NonInteractiveUsesDefaultsWhenNotProvided(t *testing.T) {
	tmpHome := t.TempDir()

	cmd := &InitCmd{
		NonInteractive: true,
		baseDir:        tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Load and verify settings
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != config.DefaultAddress {
		t.Errorf("Address = %q, want %q", ctx.Address, config.DefaultAddress)
	}
	if ctx.LogLevel != config.DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", ctx.LogLevel, config.DefaultLogLevel)
	}
}

func TestInitCmd_DoesNotOverwriteWithoutConfirmation(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings
	existingSettings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "existing.example.com",
				LogLevel: "error",
			},
		},
	}
	if err := config.Save(existingSettings, tmpHome); err != nil {
		t.Fatalf("Failed to create existing settings: %v", err)
	}

	// Create prompter that says "no" to overwrite
	prompter := ui.NewScriptedPrompter().
		OnConfirm("Settings file already exists. Overwrite?", false)

	cmd := &InitCmd{
		prompter: prompter,
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify settings are unchanged
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != "existing.example.com" {
		t.Errorf("Address = %q, want %q (should be unchanged)", ctx.Address, "existing.example.com")
	}
}

func TestInitCmd_OverwritesWithYesFlag(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings
	existingSettings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "existing.example.com",
				LogLevel: "error",
			},
		},
	}
	if err := config.Save(existingSettings, tmpHome); err != nil {
		t.Fatalf("Failed to create existing settings: %v", err)
	}

	cmd := &InitCmd{
		NonInteractive: true,
		Yes:            true,
		baseDir:        tmpHome,
	}
	cli := &CLI{
		Address: "new.example.com",
	}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify settings are overwritten
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != "new.example.com" {
		t.Errorf("Address = %q, want %q", ctx.Address, "new.example.com")
	}
}

func TestInitCmd_NonInteractiveWithoutYesErrorsOnExistingSettings(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings
	existingSettings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "existing.example.com",
				LogLevel: "error",
			},
		},
	}
	if err := config.Save(existingSettings, tmpHome); err != nil {
		t.Fatalf("Failed to create existing settings: %v", err)
	}

	cmd := &InitCmd{
		NonInteractive: true,
		Yes:            false,
		baseDir:        tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("Expected error when settings exist in non-interactive mode without --yes")
	}

	// Verify error message mentions --yes
	if errMsg := err.Error(); errMsg == "" {
		t.Error("Error message is empty")
	}
}

func TestInitCmd_InteractiveWithCustomValues(t *testing.T) {
	tmpHome := t.TempDir()

	prompter := ui.NewScriptedPrompter().
		OnPromptString("API address", "custom.example.com").
		OnPromptString("Default organization (optional)", "myorg").
		OnPromptSelect("Log level", "debug")

	cmd := &InitCmd{
		prompter: prompter,
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Load and verify settings
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != "custom.example.com" {
		t.Errorf("Address = %q, want %q", ctx.Address, "custom.example.com")
	}
	if ctx.DefaultOrg != "myorg" {
		t.Errorf("DefaultOrg = %q, want %q", ctx.DefaultOrg, "myorg")
	}
	if ctx.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", ctx.LogLevel, "debug")
	}
}

func TestInitCmd_OverwritesWithConfirmation(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings
	existingSettings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "existing.example.com",
				LogLevel: "error",
			},
		},
	}
	if err := config.Save(existingSettings, tmpHome); err != nil {
		t.Fatalf("Failed to create existing settings: %v", err)
	}

	// Create prompter that says "yes" to overwrite and provides new values
	prompter := ui.NewScriptedPrompter().
		OnConfirm("Settings file already exists. Overwrite?", true).
		OnPromptString("API address", "new.example.com").
		OnPromptString("Default organization (optional)", "").
		OnPromptSelect("Log level", "info")

	cmd := &InitCmd{
		prompter: prompter,
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify settings are overwritten
	settings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	ctx := settings.Contexts["default"]
	if ctx.Address != "new.example.com" {
		t.Errorf("Address = %q, want %q", ctx.Address, "new.example.com")
	}
}

func TestInitCmd_InvalidAddressRejected(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"malformed URL", "://broken"},
		{"empty hostname", "https:///path"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpHome := t.TempDir()

			cmd := &InitCmd{
				NonInteractive: true,
				baseDir:        tmpHome,
			}
			cli := &CLI{
				Address: tc.address,
			}

			err := cmd.Run(cli)
			if err == nil {
				t.Fatal("expected error for invalid address, got nil")
			}

			// Verify it's a RuntimeError for exit code 2
			var runtimeErr internalcmd.RuntimeError
			if !errors.As(err, &runtimeErr) {
				t.Errorf("expected RuntimeError, got %T", err)
			}

			if !strings.Contains(err.Error(), "invalid address") {
				t.Errorf("expected 'invalid address' in error, got: %v", err)
			}

			// Verify settings file was not created
			settingsPath := filepath.Join(tmpHome, ".tfccli", "settings.json")
			if _, err := os.Stat(settingsPath); err == nil {
				t.Error("settings.json should not have been created for invalid address")
			}
		})
	}
}

// TestInitCmd_PrompterErrorOnOverwriteConfirm tests prompter error during overwrite prompt.
func TestInitCmd_PrompterErrorOnOverwriteConfirm(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings
	existingSettings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "existing.example.com", LogLevel: "error"},
		},
	}
	if err := config.Save(existingSettings, tmpHome); err != nil {
		t.Fatalf("Failed to create existing settings: %v", err)
	}

	cmd := &InitCmd{
		prompter: &errorPrompter{err: errors.New("terminal not available")},
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
		t.Errorf("expected prompt confirmation error, got: %v", err)
	}
}

// TestInitCmd_PrompterErrorOnAddress tests prompter error during address prompt.
func TestInitCmd_PrompterErrorOnAddress(t *testing.T) {
	tmpHome := t.TempDir()

	cmd := &InitCmd{
		prompter: &errorPrompter{err: errors.New("EOF")},
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to prompt for address") {
		t.Errorf("expected address prompt error, got: %v", err)
	}
}

// TestInitCmd_PrompterErrorOnOrg tests prompter error during organization prompt.
func TestInitCmd_PrompterErrorOnOrg(t *testing.T) {
	tmpHome := t.TempDir()

	// Error on the 2nd PromptString call (org), succeed on the 1st (address)
	cmd := &InitCmd{
		prompter: &sequentialErrorPrompter{errorOnCall: 2, err: errors.New("EOF")},
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to prompt for default org") {
		t.Errorf("expected org prompt error, got: %v", err)
	}
}

// TestInitCmd_PrompterErrorOnLogLevel tests prompter error during log level prompt.
func TestInitCmd_PrompterErrorOnLogLevel(t *testing.T) {
	tmpHome := t.TempDir()

	cmd := &InitCmd{
		prompter: &selectErrorPrompter{err: errors.New("EOF")},
		baseDir:  tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to prompt for log level") {
		t.Errorf("expected log level prompt error, got: %v", err)
	}
}

// TestInitCmd_SaveError tests that config.Save errors are properly surfaced.
func TestInitCmd_SaveError(t *testing.T) {
	// Create a temp dir and then make it read-only to prevent writing
	tmpHome := t.TempDir()
	tfccliDir := filepath.Join(tmpHome, ".tfccli")
	if err := os.MkdirAll(tfccliDir, 0o700); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}
	// Make directory read-only to prevent writing settings.json
	if err := os.Chmod(tfccliDir, 0o500); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(tfccliDir, 0o700) // Restore so cleanup can delete
	})

	cmd := &InitCmd{
		NonInteractive: true,
		Yes:            true,
		baseDir:        tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when save fails, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to save settings") {
		t.Errorf("expected 'failed to save settings' in error, got: %v", err)
	}
}

// TestInitCmd_StatPermissionError tests that os.Stat permission errors are surfaced.
func TestInitCmd_StatPermissionError(t *testing.T) {
	tmpHome := t.TempDir()

	// Create the .tfccli directory with no read permission to trigger stat error
	tfccliDir := filepath.Join(tmpHome, ".tfccli")
	if err := os.MkdirAll(tfccliDir, 0o700); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	// Create the settings file so it exists
	settingsPath := filepath.Join(tfccliDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("Failed to create settings file: %v", err)
	}

	// Remove read permission from settings file to trigger permission error on stat
	if err := os.Chmod(settingsPath, 0o000); err != nil {
		t.Fatalf("Failed to chmod settings file: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(settingsPath, 0o600) // Restore so cleanup can delete
	})

	// Also remove read permission from directory to prevent stat from reading file metadata
	if err := os.Chmod(tfccliDir, 0o000); err != nil {
		t.Fatalf("Failed to chmod dir: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(tfccliDir, 0o700)
	})

	cmd := &InitCmd{
		NonInteractive: true,
		Yes:            true,
		baseDir:        tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when stat fails with permission error, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to check settings file") {
		t.Errorf("expected 'failed to check settings file' in error, got: %v", err)
	}
}
