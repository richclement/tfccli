package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/ui"
)

// InitCmd initializes CLI settings.
type InitCmd struct {
	NonInteractive bool   `help:"Run in non-interactive mode (for CI/agents)."`
	DefaultOrg     string `name:"default-org" help:"Default organization."`
	LogLevel       string `name:"log-level" enum:"debug,info,warn,error," default:"" help:"Log level (debug, info, warn, error)."`
	Yes            bool   `help:"Skip confirmation prompts (e.g., overwrite existing settings)."`

	// Dependencies (injectable for testing)
	prompter ui.Prompter
	baseDir  string
	stdout   io.Writer
}

// Run executes the init command.
// The cli parameter is passed via kong's Bind feature.
func (c *InitCmd) Run(cli *CLI) error {
	// Use defaults if not injected
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}

	// Check if settings already exist
	settingsPath, err := config.SettingsPath(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to determine settings path: %w", err))
	}

	settingsExist := false
	if _, err := os.Stat(settingsPath); err == nil {
		settingsExist = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to check settings file: %w", err))
	}

	// Handle existing settings
	if settingsExist {
		if !c.Yes && !c.NonInteractive {
			overwrite, err := c.prompter.Confirm("Settings file already exists. Overwrite?", false)
			if err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
			}
			if !overwrite {
				fmt.Fprintln(c.stdout, "Aborting init (settings unchanged).")
				return nil
			}
		} else if !c.Yes {
			// Non-interactive without --yes: abort
			return internalcmd.NewRuntimeError(errors.New("settings file already exists; use --yes to overwrite"))
		}
	}

	// Collect values
	var address, defaultOrg, logLevel string

	if c.NonInteractive {
		// Use flag values or defaults
		// Use global --address flag for init's address parameter
		address = cli.Address
		if address == "" {
			address = config.DefaultAddress
		}
		defaultOrg = c.DefaultOrg
		logLevel = c.LogLevel
		if logLevel == "" {
			logLevel = config.DefaultLogLevel
		}
	} else {
		// Interactive prompts
		// Use global --address as default if provided
		defaultAddress := config.DefaultAddress
		if cli.Address != "" {
			defaultAddress = cli.Address
		}
		address, err = c.prompter.PromptString("API address", defaultAddress)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for address: %w", err))
		}

		defaultOrg, err = c.prompter.PromptString("Default organization (optional)", "")
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for default org: %w", err))
		}

		logLevelOptions := []string{"debug", "info", "warn", "error"}
		logLevel, err = c.prompter.PromptSelect("Log level", logLevelOptions, config.DefaultLogLevel)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for log level: %w", err))
		}
	}

	// Validate address format
	if _, err := auth.ExtractHostname(address); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("invalid address %q: %w", address, err))
	}

	// Create settings
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    address,
				DefaultOrg: defaultOrg,
				LogLevel:   logLevel,
			},
		},
	}

	// Save settings
	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Fprintf(c.stdout, "Settings written to %s\n", settingsPath)
	return nil
}
