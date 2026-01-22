package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/ui"
)

// Helper to create settings for tests
func createTestSettings(t *testing.T, tmpHome string, settings *config.Settings) {
	t.Helper()
	if err := config.Save(settings, tmpHome); err != nil {
		t.Fatalf("Failed to create test settings: %v", err)
	}
}

func TestContextsListCmd_ListsAllContexts(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsListCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "table"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "default") {
		t.Errorf("expected 'default' in output, got: %s", out)
	}
	if !strings.Contains(out, "prod") {
		t.Errorf("expected 'prod' in output, got: %s", out)
	}
	// Verify current context is marked with asterisk
	if !strings.Contains(out, "*") {
		t.Errorf("expected '*' marker for current context, got: %s", out)
	}
}

func TestContextsAddCmd_CreatesNewContext(t *testing.T) {
	tmpHome := t.TempDir()

	// Create existing settings with just default context
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsAddCmd{
		Name:       "prod",
		CtxAddress: "tfe.example.com",
		DefaultOrg: "acme",
		LogLevel:   "warn",
		baseDir:    tmpHome,
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify the context was added
	loadedSettings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	prodCtx, exists := loadedSettings.Contexts["prod"]
	if !exists {
		t.Fatal("Context 'prod' was not added")
	}
	if prodCtx.Address != "tfe.example.com" {
		t.Errorf("Address = %q, want %q", prodCtx.Address, "tfe.example.com")
	}
	if prodCtx.DefaultOrg != "acme" {
		t.Errorf("DefaultOrg = %q, want %q", prodCtx.DefaultOrg, "acme")
	}
	if prodCtx.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", prodCtx.LogLevel, "warn")
	}
}

func TestContextsAddCmd_ErrorsIfContextExists(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsAddCmd{
		Name:       "default",
		CtxAddress: "new.example.com",
		baseDir:    tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("Expected error when adding existing context")
	}
}

func TestContextsAddCmd_InvalidAddressRejected(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	testCases := []struct {
		name    string
		address string
	}{
		{name: "malformed URL", address: "://broken"},
		{name: "empty hostname", address: "https:///path"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &ContextsAddCmd{
				Name:       "bad-context",
				CtxAddress: tc.address,
				baseDir:    tmpHome,
			}
			cli := &CLI{}

			err := cmd.Run(cli)
			if err == nil {
				t.Fatalf("expected error for invalid address %q, got nil", tc.address)
			}
			errStr := err.Error()
			if !strings.Contains(errStr, "invalid address") {
				t.Errorf("expected 'invalid address' in error, got: %v", err)
			}
		})
	}
}

// TestContextsAddCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsAddCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	cmd := &ContextsAddCmd{
		Name:       "new-context",
		CtxAddress: "tfe.example.com",
		baseDir:    tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}

func TestContextsUseCmd_SwitchesCurrentContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsUseCmd{
		Name:    "prod",
		baseDir: tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify current context was switched
	loadedSettings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if loadedSettings.CurrentContext != "prod" {
		t.Errorf("CurrentContext = %q, want %q", loadedSettings.CurrentContext, "prod")
	}
}

func TestContextsUseCmd_ErrorsIfContextNotFound(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsUseCmd{
		Name:    "nonexistent",
		baseDir: tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("Expected error when switching to nonexistent context")
	}
}

// TestContextsUseCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsUseCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	cmd := &ContextsUseCmd{
		Name:    "some-context",
		baseDir: tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}

// TestContextsUseCmd_SaveError tests that save errors are properly surfaced.
func TestContextsUseCmd_SaveError(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	// Make the settings file read-only to prevent writing
	settingsPath := filepath.Join(tmpHome, ".tfccli", "settings.json")
	if err := os.Chmod(settingsPath, 0o400); err != nil {
		t.Fatalf("Failed to chmod settings file: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(settingsPath, 0o600) // Restore so cleanup can delete
	})

	cmd := &ContextsUseCmd{
		Name:    "prod",
		baseDir: tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when save fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to save settings") {
		t.Errorf("expected save failure message, got: %v", err)
	}
}

func TestContextsRemoveCmd_RemovesContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	forceVal := true
	cmd := &ContextsRemoveCmd{
		Name:      "prod",
		baseDir:   tmpHome,
		forceFlag: &forceVal,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify the context was removed
	loadedSettings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if _, exists := loadedSettings.Contexts["prod"]; exists {
		t.Fatal("Context 'prod' should have been removed")
	}
}

func TestContextsRemoveCmd_ErrorsWhenRemovingCurrentContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	forceVal := true
	cmd := &ContextsRemoveCmd{
		Name:      "default",
		baseDir:   tmpHome,
		forceFlag: &forceVal,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("Expected error when removing current context")
	}
	// Verify error message
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Error message is empty")
	}
}

func TestContextsRemoveCmd_PromptsWithoutForce(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	prompter := ui.NewScriptedPrompter().
		OnConfirm("Remove context \"prod\"?", false)

	cmd := &ContextsRemoveCmd{
		Name:     "prod",
		baseDir:  tmpHome,
		prompter: prompter,
	}
	cli := &CLI{Force: false}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify the context was NOT removed (user said no)
	loadedSettings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if _, exists := loadedSettings.Contexts["prod"]; !exists {
		t.Fatal("Context 'prod' should NOT have been removed (user declined)")
	}
}

func TestContextsRemoveCmd_RemovesWithConfirmation(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	prompter := ui.NewScriptedPrompter().
		OnConfirm("Remove context \"prod\"?", true)

	cmd := &ContextsRemoveCmd{
		Name:     "prod",
		baseDir:  tmpHome,
		prompter: prompter,
	}
	cli := &CLI{Force: false}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify the context was removed (user confirmed)
	loadedSettings, err := config.Load(tmpHome)
	if err != nil {
		t.Fatalf("Failed to load settings: %v", err)
	}

	if _, exists := loadedSettings.Contexts["prod"]; exists {
		t.Fatal("Context 'prod' should have been removed (user confirmed)")
	}
}

// TestContextsRemoveCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsRemoveCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	forceVal := true
	cmd := &ContextsRemoveCmd{
		Name:      "some-context",
		baseDir:   tmpHome,
		forceFlag: &forceVal,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}

func TestContextsShowCmd_ShowsCurrentContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", DefaultOrg: "myorg", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "table"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "default") {
		t.Errorf("expected 'default' in output, got: %s", out)
	}
	if !strings.Contains(out, "(current)") {
		t.Errorf("expected '(current)' marker in output, got: %s", out)
	}
	if !strings.Contains(out, "myorg") {
		t.Errorf("expected 'myorg' in output, got: %s", out)
	}
	// Verify address and log level are also displayed
	if !strings.Contains(out, "app.terraform.io") {
		t.Errorf("expected 'app.terraform.io' in output, got: %s", out)
	}
	if !strings.Contains(out, "info") {
		t.Errorf("expected 'info' log level in output, got: %s", out)
	}
}

func TestContextsShowCmd_ShowsNamedContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", DefaultOrg: "acme", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		Name:        "prod",
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "table"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "prod") {
		t.Errorf("expected 'prod' in output, got: %s", out)
	}
	if !strings.Contains(out, "acme") {
		t.Errorf("expected 'acme' in output, got: %s", out)
	}
	// Verify address and log level are also displayed
	if !strings.Contains(out, "tfe.example.com") {
		t.Errorf("expected 'tfe.example.com' in output, got: %s", out)
	}
	if !strings.Contains(out, "warn") {
		t.Errorf("expected 'warn' log level in output, got: %s", out)
	}
	// Verify non-current context is NOT marked as current
	if strings.Contains(out, "(current)") {
		t.Errorf("expected non-current context to NOT have '(current)' marker, got: %s", out)
	}
}

func TestContextsShowCmd_ErrorsIfContextNotFound(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		Name:        "nonexistent",
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("Expected error when showing nonexistent context")
	}
}

// TestContextsListCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsListCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	var buf bytes.Buffer
	cmd := &ContextsListCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}

// TestContextsShowCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsShowCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}

// TestContextsRemoveCmd_ContextNotFound tests error when removing nonexistent context.
func TestContextsRemoveCmd_ContextNotFound(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	forceVal := true
	cmd := &ContextsRemoveCmd{
		Name:      "nonexistent",
		baseDir:   tmpHome,
		forceFlag: &forceVal,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when context not found, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// TestContextsRemoveCmd_PrompterError tests that prompter errors are surfaced.
func TestContextsRemoveCmd_PrompterError(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsRemoveCmd{
		Name:     "prod",
		baseDir:  tmpHome,
		prompter: &errorPrompter{err: errors.New("terminal not available")},
	}
	cli := &CLI{Force: false}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
		t.Errorf("expected prompt error, got: %v", err)
	}
}

// TestContextsAddCmd_SaveError tests that save errors are properly surfaced.
func TestContextsAddCmd_SaveError(t *testing.T) {
	tmpHome := t.TempDir()

	// Create initial settings
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	// Make the settings file read-only to prevent writing
	settingsPath := filepath.Join(tmpHome, ".tfccli", "settings.json")
	if err := os.Chmod(settingsPath, 0o400); err != nil {
		t.Fatalf("Failed to chmod settings file: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(settingsPath, 0o600) // Restore so cleanup can delete
	})

	cmd := &ContextsAddCmd{
		Name:       "new-context",
		CtxAddress: "tfe.example.com",
		baseDir:    tmpHome,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when save fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to save settings") {
		t.Errorf("expected save failure message, got: %v", err)
	}
}

// TestContextsRemoveCmd_SaveError tests that save errors are properly surfaced.
func TestContextsRemoveCmd_SaveError(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	// Make the settings file read-only to prevent writing
	settingsPath := filepath.Join(tmpHome, ".tfccli", "settings.json")
	if err := os.Chmod(settingsPath, 0o400); err != nil {
		t.Fatalf("Failed to chmod settings file: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(settingsPath, 0o600) // Restore so cleanup can delete
	})

	forceVal := true
	cmd := &ContextsRemoveCmd{
		Name:      "prod",
		baseDir:   tmpHome,
		forceFlag: &forceVal,
	}
	cli := &CLI{}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when save fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to save settings") {
		t.Errorf("expected save failure message, got: %v", err)
	}
}

// TestContextsListCmd_JSONOutput tests JSON output format for list command.
func TestContextsListCmd_JSONOutput(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
			"prod":    {Address: "tfe.example.com", LogLevel: "warn"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsListCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var items []contextListItem
	if err := json.Unmarshal(buf.Bytes(), &items); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	// Verify items are sorted alphabetically
	if items[0].Name != "default" {
		t.Errorf("expected first item 'default', got %q", items[0].Name)
	}
	if !items[0].IsCurrent {
		t.Error("expected 'default' to be marked as current")
	}
	if items[1].Name != "prod" {
		t.Errorf("expected second item 'prod', got %q", items[1].Name)
	}
	if items[1].IsCurrent {
		t.Error("expected 'prod' to NOT be marked as current")
	}
}

// TestContextsShowCmd_JSONOutput tests JSON output format for show command.
func TestContextsShowCmd_JSONOutput(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", DefaultOrg: "acme", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var item contextShowItem
	if err := json.Unmarshal(buf.Bytes(), &item); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if item.Name != "default" {
		t.Errorf("expected name 'default', got %q", item.Name)
	}
	if !item.IsCurrent {
		t.Error("expected is_current to be true")
	}
	if item.Address != "app.terraform.io" {
		t.Errorf("expected address 'app.terraform.io', got %q", item.Address)
	}
	if item.DefaultOrg != "acme" {
		t.Errorf("expected default_org 'acme', got %q", item.DefaultOrg)
	}
	if item.LogLevel != "info" {
		t.Errorf("expected log_level 'info', got %q", item.LogLevel)
	}
}

// TestContextsShowCmd_EmptyDefaultOrgDisplayed tests that empty default_org shows "(none)" in table output.
func TestContextsShowCmd_EmptyDefaultOrgDisplayed(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	var buf bytes.Buffer
	cmd := &ContextsShowCmd{
		baseDir:     tmpHome,
		stdout:      &buf,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "table"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for empty default_org, got: %s", out)
	}
}
