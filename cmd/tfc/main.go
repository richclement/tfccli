package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
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

	Version            VersionCmd            `cmd:"" help:"Print version information."`
	Doctor             DoctorCmd             `cmd:"" help:"Validate settings, token discovery, and connectivity."`
	Init               InitCmd               `cmd:"" help:"Initialize CLI settings."`
	Contexts           ContextsCmd           `cmd:"" help:"Manage named contexts."`
	Organizations      OrganizationsCmd      `cmd:"" help:"Manage organizations."`
	Projects           ProjectsCmd           `cmd:"" help:"Manage projects."`
	Workspaces         WorkspacesCmd         `cmd:"" help:"Manage workspaces."`
	WorkspaceVariables WorkspaceVariablesCmd `cmd:"" name:"workspace-variables" help:"Manage workspace variables."`
	WorkspaceResources WorkspaceResourcesCmd `cmd:"" name:"workspace-resources" help:"List workspace resources."`
	Runs               RunsCmd               `cmd:"" help:"Manage runs."`
	Plans              PlansCmd              `cmd:"" help:"Manage plans."`
	Applies            AppliesCmd            `cmd:"" help:"Manage applies."`
}

// VersionCmd prints the CLI version info.
type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	fmt.Printf("version: %s\n", version)
	fmt.Printf("commit:  %s\n", commit)
	fmt.Printf("date:    %s\n", date)
	return nil
}

// DoctorCmd validates settings, token discovery, and connectivity.
type DoctorCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        *os.File
	clientFactory func(cfg tfcapi.ClientConfig) (doctorClient, error)
}

// doctorClient abstracts the TFC client for testing.
type doctorClient interface {
	Ping(ctx context.Context) error
}

// DoctorCheck represents a single doctor check result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DoctorResult represents the full doctor output.
type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
}

func (d *DoctorCmd) Run(cli *CLI) error {
	// Set up defaults
	if d.tokenResolver == nil {
		d.tokenResolver = auth.NewTokenResolver()
	}
	if d.ttyDetector == nil {
		d.ttyDetector = &output.RealTTYDetector{}
	}
	if d.stdout == nil {
		d.stdout = os.Stdout
	}
	if d.clientFactory == nil {
		d.clientFactory = defaultClientFactory
	}

	isTTY := d.ttyDetector.IsTTY(d.stdout)
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	result := &DoctorResult{Checks: make([]DoctorCheck, 0)}
	hasFailure := false

	// Check 1: Settings file exists and is valid
	settings, err := config.Load(d.baseDir)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "settings",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("run 'tfc init': %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "settings",
		Status: string(output.StatusPass),
		Detail: "settings.json loaded",
	})

	// Resolve context (flag override or current)
	contextName := cli.Context
	if contextName == "" {
		contextName = settings.CurrentContext
	}
	ctx, exists := settings.Contexts[contextName]
	if !exists {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "context",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("context %q not found", contextName),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}

	// Apply defaults and overrides
	resolved := ctx.WithDefaults()
	if cli.Address != "" {
		resolved.Address = cli.Address
	}

	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "context",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("using context %q", contextName),
	})

	// Check 2: Address parsing
	hostname, err := auth.ExtractHostname(resolved.Address)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "address",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("invalid address %q: %v", resolved.Address, err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "address",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("hostname: %s", hostname),
	})

	// Check 3: Token resolution
	tokenResult, err := d.tokenResolver.ResolveToken(resolved.Address)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "token",
			Status: string(output.StatusFail),
			Detail: err.Error(),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "token",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("source: %s", tokenResult.Source),
	})

	// Check 4: Connectivity
	client, err := d.clientFactory(tfcapi.ClientConfig{
		Address: resolved.Address,
		Token:   tokenResult.Token,
	})
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "connectivity",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("failed to create client: %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}

	pingCtx := context.Background()
	if err := client.Ping(pingCtx); err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "connectivity",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("API check failed: %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "connectivity",
		Status: string(output.StatusPass),
		Detail: "API reachable",
	})

	return d.outputAndError(result, format, isTTY, hasFailure)
}

func (d *DoctorCmd) outputAndError(result *DoctorResult, format output.Format, isTTY bool, hasFailure bool) error {
	if format == output.FormatJSON {
		if err := output.WriteJSON(d.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(d.stdout, []string{"CHECK", "STATUS", "DETAIL"}, isTTY)
		for _, check := range result.Checks {
			tw.AddRow(check.Name, output.StatusStyle(output.Status(check.Status), isTTY), check.Detail)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	if hasFailure {
		return internalcmd.NewRuntimeError(errors.New("doctor checks failed"))
	}
	return nil
}

// defaultClientFactory creates a real TFC client that satisfies doctorClient.
func defaultClientFactory(cfg tfcapi.ClientConfig) (doctorClient, error) {
	return tfcapi.NewClientWithWrapper(cfg)
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

// ContextsCmd manages named contexts.
type ContextsCmd struct {
	List   ContextsListCmd   `cmd:"" help:"List all contexts."`
	Add    ContextsAddCmd    `cmd:"" help:"Add a new context."`
	Use    ContextsUseCmd    `cmd:"" help:"Switch to a different context."`
	Remove ContextsRemoveCmd `cmd:"" help:"Remove a context."`
	Show   ContextsShowCmd   `cmd:"" help:"Show context configuration."`
}

// ContextsListCmd lists all contexts.
type ContextsListCmd struct {
	baseDir string
}

func (c *ContextsListCmd) Run() error {
	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	for name := range settings.Contexts {
		marker := "  "
		if name == settings.CurrentContext {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, name)
	}
	return nil
}

// ContextsAddCmd adds a new context.
type ContextsAddCmd struct {
	Name       string `arg:"" help:"Name for the new context."`
	CtxAddress string `name:"ctx-address" required:"" help:"API address for the context."`
	DefaultOrg string `name:"default-org" help:"Default organization."`
	LogLevel   string `name:"log-level" enum:"debug,info,warn,error," default:"" help:"Log level."`

	baseDir string
}

func (c *ContextsAddCmd) Run() error {
	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if _, exists := settings.Contexts[c.Name]; exists {
		return internalcmd.NewRuntimeError(fmt.Errorf("context %q already exists", c.Name))
	}

	settings.Contexts[c.Name] = config.Context{
		Address:    c.CtxAddress,
		DefaultOrg: c.DefaultOrg,
		LogLevel:   c.LogLevel,
	}

	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Printf("Context %q added.\n", c.Name)
	return nil
}

// ContextsUseCmd switches to a different context.
type ContextsUseCmd struct {
	Name string `arg:"" help:"Name of the context to switch to."`

	baseDir string
}

func (c *ContextsUseCmd) Run() error {
	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if _, exists := settings.Contexts[c.Name]; !exists {
		return internalcmd.NewRuntimeError(fmt.Errorf("context %q not found", c.Name))
	}

	settings.CurrentContext = c.Name

	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Printf("Switched to context %q.\n", c.Name)
	return nil
}

// ContextsRemoveCmd removes a context.
type ContextsRemoveCmd struct {
	Name string `arg:"" help:"Name of the context to remove."`

	baseDir   string
	prompter  ui.Prompter
	forceFlag *bool // Pointer to allow injection from parent CLI
}

func (c *ContextsRemoveCmd) Run(cli *CLI) error {
	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if _, exists := settings.Contexts[c.Name]; !exists {
		return internalcmd.NewRuntimeError(fmt.Errorf("context %q not found", c.Name))
	}

	if c.Name == settings.CurrentContext {
		return internalcmd.NewRuntimeError(errors.New("cannot remove current context; switch to another context first"))
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm removal unless --force
	if !force {
		if c.prompter == nil {
			c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
		}
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Remove context %q?", c.Name), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Println("Aborting removal.")
			return nil
		}
	}

	delete(settings.Contexts, c.Name)

	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Printf("Context %q removed.\n", c.Name)
	return nil
}

// ContextsShowCmd shows context configuration.
type ContextsShowCmd struct {
	Name string `arg:"" optional:"" help:"Name of the context to show (defaults to current)."`

	baseDir string
}

func (c *ContextsShowCmd) Run() error {
	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	name := c.Name
	if name == "" {
		name = settings.CurrentContext
	}

	ctx, exists := settings.Contexts[name]
	if !exists {
		return internalcmd.NewRuntimeError(fmt.Errorf("context %q not found", name))
	}

	// Apply defaults for display
	resolved := ctx.WithDefaults()

	current := ""
	if name == settings.CurrentContext {
		current = " (current)"
	}

	fmt.Printf("Context: %s%s\n", name, current)
	fmt.Printf("  Address:     %s\n", resolved.Address)
	fmt.Printf("  Default Org: %s\n", resolved.DefaultOrg)
	fmt.Printf("  Log Level:   %s\n", resolved.LogLevel)
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
