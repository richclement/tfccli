package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/ui"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run())
}

type CLI struct {
	Context      string `help:"Select a named context from settings."`
	Address      string `help:"Override the API address for this invocation."`
	Org          string `help:"Override the default organization for this invocation."`
	OutputFormat string `name:"output-format" enum:"table,json," default:"" help:"Output format: table or json."`
	Debug        bool   `help:"Enable debug logging for this invocation."`
	Force        bool   `help:"Bypass confirmation prompts for destructive operations."`

	Version VersionCmd `cmd:"" help:"Print version information."`
	Doctor  DoctorCmd  `cmd:"" help:"Validate settings, token discovery, and connectivity."`
	Init    InitCmd    `cmd:"" help:"Initialize CLI settings."`
}

// VersionCmd prints the CLI version info.
type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	fmt.Printf("version: %s\n", version)
	fmt.Printf("commit:  %s\n", commit)
	fmt.Printf("date:    %s\n", date)
	return nil
}

// DoctorCmd is a placeholder for the full doctor implementation.
type DoctorCmd struct{}

func (d *DoctorCmd) Run() error {
	// Placeholder - full implementation in Task 14
	return internalcmd.NewRuntimeError(errors.New("doctor not yet implemented"))
}

// InitCmd initializes CLI settings.
type InitCmd struct {
	NonInteractive bool   `help:"Run in non-interactive mode (for CI/agents)."`
	DefaultOrg     string `name:"default-org" help:"Default organization."`
	LogLevel       string `name:"log-level" enum:"debug,info,warn,error," default:"" help:"Log level (debug, info, warn, error)."`
	Yes            bool   `help:"Skip confirmation prompts (e.g., overwrite existing settings)."`

	// Dependencies (injectable for testing)
	prompter ui.Prompter
	baseDir  string
}

// Run executes the init command.
// The cli parameter is passed via kong's Bind feature.
func (c *InitCmd) Run(cli *CLI) error {
	// Use defaults if not injected
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Check if settings already exist
	settingsPath, err := config.SettingsPath(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to determine settings path: %w", err))
	}

	settingsExist := false
	if _, err := os.Stat(settingsPath); err == nil {
		settingsExist = true
	}

	// Handle existing settings
	if settingsExist {
		if !c.Yes && !c.NonInteractive {
			overwrite, err := c.prompter.Confirm("Settings file already exists. Overwrite?", false)
			if err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
			}
			if !overwrite {
				fmt.Println("Aborting init (settings unchanged).")
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

	fmt.Printf("Settings written to %s\n", settingsPath)
	return nil
}

type exitError struct {
	code int
}

func run() (exitCode int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if exitErr, ok := recovered.(exitError); ok {
				exitCode = exitErr.code
				return
			}
			fmt.Fprintln(os.Stderr, "unexpected error")
			exitCode = 3
		}
	}()

	cli := CLI{}
	parser, err := kong.New(
		&cli,
		kong.Name("tfc"),
		kong.Description("Terraform Cloud API CLI"),
		kong.Exit(func(code int) {
			panic(exitError{code: code})
		}),
		kong.Bind(&cli),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		printParseError(err)
		return 1
	}

	if err := ctx.Run(); err != nil {
		return exitCodeForError(err)
	}
	return 0
}

func printParseError(err error) {
	fmt.Fprintln(os.Stderr, err)
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		_ = parseErr.Context.PrintUsage(true)
	}
}

func exitCodeForError(err error) int {
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		fmt.Fprintln(os.Stderr, runtimeErr.Error())
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 3
}
