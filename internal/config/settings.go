package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultAddress is the default Terraform Cloud API address.
const DefaultAddress = "app.terraform.io"

// DefaultLogLevel is the default logging level.
const DefaultLogLevel = "info"

// ValidLogLevels enumerates acceptable log_level values.
var ValidLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Context represents a named configuration context.
type Context struct {
	Address    string `json:"address,omitempty"`
	DefaultOrg string `json:"default_org,omitempty"`
	LogLevel   string `json:"log_level,omitempty"`
}

// Settings represents the CLI configuration stored in settings.json.
type Settings struct {
	CurrentContext string             `json:"current_context"`
	Contexts       map[string]Context `json:"contexts"`
}

// SettingsDir returns the settings directory path.
// If baseDir is empty, uses the user's home directory.
func SettingsDir(baseDir string) (string, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		baseDir = home
	}
	return filepath.Join(baseDir, ".tfccli"), nil
}

// SettingsPath returns the full path to settings.json.
// If baseDir is empty, uses the user's home directory.
func SettingsPath(baseDir string) (string, error) {
	dir, err := SettingsDir(baseDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Load reads and validates settings from settings.json.
// If baseDir is empty, uses the user's home directory.
func Load(baseDir string) (*Settings, error) {
	path, err := SettingsPath(baseDir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("settings file not found: run 'tfccli init' to create one")
		}
		return nil, fmt.Errorf("cannot read settings file: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid settings file: %w", err)
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &s, nil
}

// Save writes settings to settings.json.
// If baseDir is empty, uses the user's home directory.
func Save(s *Settings, baseDir string) error {
	if err := s.Validate(); err != nil {
		return err
	}

	dir, err := SettingsDir(baseDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create settings directory: %w", err)
	}

	path, err := SettingsPath(baseDir)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("cannot write settings file: %w", err)
	}

	return nil
}

// Validate checks that the settings are well-formed.
func (s *Settings) Validate() error {
	if s.Contexts == nil {
		return fmt.Errorf("invalid settings: contexts is nil")
	}

	if s.CurrentContext == "" {
		return fmt.Errorf("invalid settings: current_context is empty")
	}

	if _, exists := s.Contexts[s.CurrentContext]; !exists {
		return fmt.Errorf("invalid settings: current context %q not found in contexts", s.CurrentContext)
	}

	for name, ctx := range s.Contexts {
		if err := ctx.Validate(); err != nil {
			return fmt.Errorf("invalid settings: context %q: %w", name, err)
		}
	}

	return nil
}

// Validate checks that the context is well-formed.
func (c *Context) Validate() error {
	if c.LogLevel != "" && !ValidLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log_level %q: must be one of debug, info, warn, error", c.LogLevel)
	}
	return nil
}

// GetCurrentContext returns the current context, applying defaults.
func (s *Settings) GetCurrentContext() Context {
	ctx := s.Contexts[s.CurrentContext]
	return ctx.WithDefaults()
}

// WithDefaults returns a copy of the context with defaults applied.
func (c Context) WithDefaults() Context {
	result := c
	if result.Address == "" {
		result.Address = DefaultAddress
	}
	if result.LogLevel == "" {
		result.LogLevel = DefaultLogLevel
	}
	return result
}

// NewDefaultSettings creates a new settings with a default context.
func NewDefaultSettings() *Settings {
	return &Settings{
		CurrentContext: "default",
		Contexts: map[string]Context{
			"default": {
				Address:  DefaultAddress,
				LogLevel: DefaultLogLevel,
			},
		},
	}
}
