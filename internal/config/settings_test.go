package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	// Gherkin: Load returns default error when file missing
	tmpDir := t.TempDir()
	_, err := Load(tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "run 'tfccli init'") {
		t.Errorf("expected error to contain %q, got %q", "run 'tfccli init'", err.Error())
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	// Gherkin: Invalid log level fails validation
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".tfccli")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	data := `{
		"current_context": "default",
		"contexts": {
			"default": {
				"address": "app.terraform.io",
				"log_level": "loud"
			}
		}
	}`
	if err := os.WriteFile(settingsFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid log_level") {
		t.Errorf("expected error to contain %q, got %q", "invalid log_level", err.Error())
	}
}

func TestLoad_MissingCurrentContext(t *testing.T) {
	// Gherkin: Current context must exist
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".tfccli")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	data := `{
		"current_context": "missing",
		"contexts": {
			"default": {
				"address": "app.terraform.io",
				"log_level": "info"
			}
		}
	}`
	if err := os.WriteFile(settingsFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmpDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "current context") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain 'current context' and 'not found', got %q", err.Error())
	}
}

func TestLoad_ValidSettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, ".tfccli")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	data := `{
		"current_context": "default",
		"contexts": {
			"default": {
				"address": "app.terraform.io/eu",
				"default_org": "acme",
				"log_level": "debug"
			}
		}
	}`
	if err := os.WriteFile(settingsFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.CurrentContext != "default" {
		t.Errorf("expected current_context=%q, got %q", "default", s.CurrentContext)
	}
	ctx := s.Contexts["default"]
	if ctx.Address != "app.terraform.io/eu" {
		t.Errorf("expected address=%q, got %q", "app.terraform.io/eu", ctx.Address)
	}
	if ctx.DefaultOrg != "acme" {
		t.Errorf("expected default_org=%q, got %q", "acme", ctx.DefaultOrg)
	}
	if ctx.LogLevel != "debug" {
		t.Errorf("expected log_level=%q, got %q", "debug", ctx.LogLevel)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewDefaultSettings()

	if err := Save(s, tmpDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	settingsFile := filepath.Join(tmpDir, ".tfccli", "settings.json")
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		t.Fatal("settings file was not created")
	}
}

func TestSave_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	original := &Settings{
		CurrentContext: "prod",
		Contexts: map[string]Context{
			"prod": {
				Address:    "tfe.example.com",
				DefaultOrg: "myorg",
				LogLevel:   "warn",
			},
		},
	}

	if err := Save(original, tmpDir); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if loaded.CurrentContext != original.CurrentContext {
		t.Errorf("current_context mismatch: got %q, want %q", loaded.CurrentContext, original.CurrentContext)
	}
	ctx := loaded.Contexts["prod"]
	if ctx.Address != "tfe.example.com" {
		t.Errorf("address mismatch: got %q, want %q", ctx.Address, "tfe.example.com")
	}
	if ctx.DefaultOrg != "myorg" {
		t.Errorf("default_org mismatch: got %q, want %q", ctx.DefaultOrg, "myorg")
	}
	if ctx.LogLevel != "warn" {
		t.Errorf("log_level mismatch: got %q, want %q", ctx.LogLevel, "warn")
	}
}

func TestValidate_EmptyCurrentContext(t *testing.T) {
	s := &Settings{
		CurrentContext: "",
		Contexts: map[string]Context{
			"default": {LogLevel: "info"},
		},
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for empty current_context")
	}
}

func TestValidate_NilContexts(t *testing.T) {
	s := &Settings{
		CurrentContext: "default",
		Contexts:       nil,
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for nil contexts")
	}
}

func TestContext_WithDefaults(t *testing.T) {
	empty := Context{}
	withDefaults := empty.WithDefaults()
	if withDefaults.Address != DefaultAddress {
		t.Errorf("expected address=%q, got %q", DefaultAddress, withDefaults.Address)
	}
	if withDefaults.LogLevel != DefaultLogLevel {
		t.Errorf("expected log_level=%q, got %q", DefaultLogLevel, withDefaults.LogLevel)
	}

	// Existing values should not be overwritten
	custom := Context{Address: "custom.example.com", LogLevel: "warn"}
	customDefaults := custom.WithDefaults()
	if customDefaults.Address != "custom.example.com" {
		t.Errorf("expected address=%q, got %q", "custom.example.com", customDefaults.Address)
	}
	if customDefaults.LogLevel != "warn" {
		t.Errorf("expected log_level=%q, got %q", "warn", customDefaults.LogLevel)
	}
}

func TestGetCurrentContext(t *testing.T) {
	s := &Settings{
		CurrentContext: "test",
		Contexts: map[string]Context{
			"test": {DefaultOrg: "myorg"},
		},
	}
	ctx := s.GetCurrentContext()
	if ctx.DefaultOrg != "myorg" {
		t.Errorf("expected default_org=%q, got %q", "myorg", ctx.DefaultOrg)
	}
	if ctx.Address != DefaultAddress {
		t.Errorf("expected default address=%q, got %q", DefaultAddress, ctx.Address)
	}
	if ctx.LogLevel != DefaultLogLevel {
		t.Errorf("expected default log_level=%q, got %q", DefaultLogLevel, ctx.LogLevel)
	}
}

func TestNewDefaultSettings(t *testing.T) {
	s := NewDefaultSettings()
	if s.CurrentContext != "default" {
		t.Errorf("expected current_context=%q, got %q", "default", s.CurrentContext)
	}
	ctx, ok := s.Contexts["default"]
	if !ok {
		t.Fatal("expected 'default' context to exist")
	}
	if ctx.Address != DefaultAddress {
		t.Errorf("expected address=%q, got %q", DefaultAddress, ctx.Address)
	}
	if ctx.LogLevel != DefaultLogLevel {
		t.Errorf("expected log_level=%q, got %q", DefaultLogLevel, ctx.LogLevel)
	}
}

func TestValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}
	for _, level := range validLevels {
		ctx := Context{LogLevel: level}
		if err := ctx.Validate(); err != nil {
			t.Errorf("log_level %q should be valid, got error: %v", level, err)
		}
	}

	invalidLevels := []string{"loud", "trace", "fatal", "DEBUG", "INFO"}
	for _, level := range invalidLevels {
		ctx := Context{LogLevel: level}
		if err := ctx.Validate(); err == nil {
			t.Errorf("log_level %q should be invalid", level)
		}
	}
}
