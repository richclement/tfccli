package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/ui"
)

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
	baseDir     string
	stdout      io.Writer
	ttyDetector output.TTYDetector
}

// contextListItem represents a context in JSON output.
type contextListItem struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
}

func (c *ContextsListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}

	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	// Collect and sort context names for deterministic output
	names := make([]string, 0, len(settings.Contexts))
	for name := range settings.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		items := make([]contextListItem, 0, len(names))
		for _, name := range names {
			items = append(items, contextListItem{
				Name:      name,
				IsCurrent: name == settings.CurrentContext,
			})
		}
		if err := output.WriteJSON(c.stdout, items); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
		return nil
	}

	// Table output
	if len(names) == 0 {
		fmt.Fprintln(c.stdout, "No contexts found.")
		return nil
	}
	tw := output.NewTableWriter(c.stdout, []string{"", "NAME"}, isTTY)
	for _, name := range names {
		marker := ""
		if name == settings.CurrentContext {
			marker = "*"
		}
		tw.AddRow(marker, name)
	}
	if _, err := tw.Render(); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
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
	stdout  io.Writer
}

func (c *ContextsAddCmd) Run(cli *CLI) error {
	// Set defaults
	if c.stdout == nil {
		c.stdout = os.Stdout
	}

	settings, err := config.Load(c.baseDir)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if _, exists := settings.Contexts[c.Name]; exists {
		return internalcmd.NewRuntimeError(fmt.Errorf("context %q already exists", c.Name))
	}

	// Validate address format
	if _, err := auth.ExtractHostname(c.CtxAddress); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("invalid address %q: %w", c.CtxAddress, err))
	}

	settings.Contexts[c.Name] = config.Context{
		Address:    c.CtxAddress,
		DefaultOrg: c.DefaultOrg,
		LogLevel:   c.LogLevel,
	}

	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Fprintf(c.stdout, "Context %q added.\n", c.Name)
	return nil
}

// ContextsUseCmd switches to a different context.
type ContextsUseCmd struct {
	Name string `arg:"" help:"Name of the context to switch to."`

	baseDir string
	stdout  io.Writer
}

func (c *ContextsUseCmd) Run(cli *CLI) error {
	// Set defaults
	if c.stdout == nil {
		c.stdout = os.Stdout
	}

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

	fmt.Fprintf(c.stdout, "Switched to context %q.\n", c.Name)
	return nil
}

// ContextsRemoveCmd removes a context.
type ContextsRemoveCmd struct {
	Name string `arg:"" help:"Name of the context to remove."`

	baseDir   string
	prompter  ui.Prompter
	stdout    io.Writer
	forceFlag *bool // Pointer to allow injection from parent CLI
}

func (c *ContextsRemoveCmd) Run(cli *CLI) error {
	// Set defaults
	if c.stdout == nil {
		c.stdout = os.Stdout
	}

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
			fmt.Fprintln(c.stdout, "Aborting removal.")
			return nil
		}
	}

	delete(settings.Contexts, c.Name)

	if err := config.Save(settings, c.baseDir); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to save settings: %w", err))
	}

	fmt.Fprintf(c.stdout, "Context %q removed.\n", c.Name)
	return nil
}

// ContextsShowCmd shows context configuration.
type ContextsShowCmd struct {
	Name string `arg:"" optional:"" help:"Name of the context to show (defaults to current)."`

	baseDir     string
	stdout      io.Writer
	ttyDetector output.TTYDetector
}

// contextShowItem represents a context detail in JSON output.
type contextShowItem struct {
	Name       string `json:"name"`
	IsCurrent  bool   `json:"is_current"`
	Address    string `json:"address"`
	DefaultOrg string `json:"default_org"`
	LogLevel   string `json:"log_level"`
}

func (c *ContextsShowCmd) Run(cli *CLI) error {
	// Set defaults
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}

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
	isCurrent := name == settings.CurrentContext

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		item := contextShowItem{
			Name:       name,
			IsCurrent:  isCurrent,
			Address:    resolved.Address,
			DefaultOrg: resolved.DefaultOrg,
			LogLevel:   resolved.LogLevel,
		}
		if err := output.WriteJSON(c.stdout, item); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
		return nil
	}

	// Table-like text output
	current := ""
	if isCurrent {
		current = " (current)"
	}
	fmt.Fprintf(c.stdout, "Context: %s%s\n", name, current)
	fmt.Fprintf(c.stdout, "  Address:     %s\n", resolved.Address)
	defaultOrg := resolved.DefaultOrg
	if defaultOrg == "" {
		defaultOrg = "(none)"
	}
	fmt.Fprintf(c.stdout, "  Default Org: %s\n", defaultOrg)
	fmt.Fprintf(c.stdout, "  Log Level:   %s\n", resolved.LogLevel)
	return nil
}
