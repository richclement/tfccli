package main

import (
	"strings"
	"testing"

	"github.com/richclement/tfccli/internal/config"
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

	cmd := &ContextsListCmd{baseDir: tmpHome}

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Output verification would require capturing stdout
	// but the test verifies no error occurs with valid settings
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

	err := cmd.Run()
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

	err := cmd.Run()
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

			err := cmd.Run()
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

	err := cmd.Run()
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

	err := cmd.Run()
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

	err := cmd.Run()
	if err == nil {
		t.Fatal("Expected error when switching to nonexistent context")
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

func TestContextsShowCmd_ShowsCurrentContext(t *testing.T) {
	tmpHome := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {Address: "app.terraform.io", DefaultOrg: "myorg", LogLevel: "info"},
		},
	}
	createTestSettings(t, tmpHome, settings)

	cmd := &ContextsShowCmd{
		baseDir: tmpHome,
	}

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Output verification would require capturing stdout
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

	cmd := &ContextsShowCmd{
		Name:    "prod",
		baseDir: tmpHome,
	}

	err := cmd.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
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

	cmd := &ContextsShowCmd{
		Name:    "nonexistent",
		baseDir: tmpHome,
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("Expected error when showing nonexistent context")
	}
}

// TestContextsListCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsListCmd_NoSettings(t *testing.T) {
	tmpHome := t.TempDir()
	// Don't create settings file

	cmd := &ContextsListCmd{baseDir: tmpHome}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error when settings not found, got nil")
	}
}
