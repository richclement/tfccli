package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
	"github.com/richclement/tfccli/internal/ui"
)

// RunsCmd groups all runs subcommands.
type RunsCmd struct {
	List        RunsListCmd        `cmd:"" help:"List runs for a workspace."`
	Get         RunsGetCmd         `cmd:"" help:"Get a run by ID."`
	Create      RunsCreateCmd      `cmd:"" help:"Create a new run."`
	Apply       RunsApplyCmd       `cmd:"" help:"Apply a run."`
	Discard     RunsDiscardCmd     `cmd:"" help:"Discard a run."`
	Cancel      RunsCancelCmd      `cmd:"" help:"Cancel a run."`
	ForceCancel RunsForceCancelCmd `cmd:"" name:"force-cancel" help:"Force-cancel a run."`
}

// runJSON is a JSON-serializable representation of a run.
type runJSON struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	CreatedAt   string `json:"created_at"`
	Source      string `json:"source,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// toRunJSON converts a tfe.Run to a JSON-serializable form.
func toRunJSON(run *tfe.Run) *runJSON {
	r := &runJSON{
		ID:        run.ID,
		Status:    string(run.Status),
		Message:   run.Message,
		CreatedAt: run.CreatedAt.Format(time.RFC3339),
		Source:    string(run.Source),
	}
	if run.Workspace != nil {
		r.WorkspaceID = run.Workspace.ID
	}
	return r
}

// toRunJSONList converts a slice of tfe.Run to JSON-serializable form.
func toRunJSONList(runs []*tfe.Run) []*runJSON {
	result := make([]*runJSON, len(runs))
	for i, run := range runs {
		result[i] = toRunJSON(run)
	}
	return result
}

// runsClient abstracts the TFC runs API for testing.
type runsClient interface {
	List(ctx context.Context, workspaceID string, opts *tfe.RunListOptions, limit int) ([]*tfe.Run, error)
	Read(ctx context.Context, runID string) (*tfe.Run, error)
	Create(ctx context.Context, opts tfe.RunCreateOptions) (*tfe.Run, error)
	Apply(ctx context.Context, runID string, opts tfe.RunApplyOptions) error
	Discard(ctx context.Context, runID string, opts tfe.RunDiscardOptions) error
	Cancel(ctx context.Context, runID string, opts tfe.RunCancelOptions) error
	ForceCancel(ctx context.Context, runID string, opts tfe.RunForceCancelOptions) error
}

// runsClientFactory creates a runsClient from config.
type runsClientFactory func(cfg tfcapi.ClientConfig) (runsClient, error)

// realRunsClient wraps a tfe.Client to implement runsClient with pagination.
type realRunsClient struct {
	client *tfe.Client
}

func (c *realRunsClient) List(ctx context.Context, workspaceID string, opts *tfe.RunListOptions, limit int) ([]*tfe.Run, error) {
	return tfcapi.CollectRunsWithLimit(ctx, c.client, workspaceID, opts, limit)
}

func (c *realRunsClient) Read(ctx context.Context, runID string) (*tfe.Run, error) {
	return c.client.Runs.Read(ctx, runID)
}

func (c *realRunsClient) Create(ctx context.Context, opts tfe.RunCreateOptions) (*tfe.Run, error) {
	return c.client.Runs.Create(ctx, opts)
}

func (c *realRunsClient) Apply(ctx context.Context, runID string, opts tfe.RunApplyOptions) error {
	return c.client.Runs.Apply(ctx, runID, opts)
}

func (c *realRunsClient) Discard(ctx context.Context, runID string, opts tfe.RunDiscardOptions) error {
	return c.client.Runs.Discard(ctx, runID, opts)
}

func (c *realRunsClient) Cancel(ctx context.Context, runID string, opts tfe.RunCancelOptions) error {
	return c.client.Runs.Cancel(ctx, runID, opts)
}

func (c *realRunsClient) ForceCancel(ctx context.Context, runID string, opts tfe.RunForceCancelOptions) error {
	return c.client.Runs.ForceCancel(ctx, runID, opts)
}

// defaultRunsClientFactory creates a real TFC client that satisfies runsClient.
func defaultRunsClientFactory(cfg tfcapi.ClientConfig) (runsClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realRunsClient{client: client}, nil
}

// RunsListCmd lists runs for a workspace.
type RunsListCmd struct {
	WorkspaceID string `name:"workspace-id" required:"" help:"ID of the workspace."`
	Limit       int    `help:"Maximum number of runs to return (0 = all)." default:"0"`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
}

func (c *RunsListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	runs, err := client.List(ctx, c.WorkspaceID, nil, c.Limit)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list runs: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list runs: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toRunJSONList(runs)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		if len(runs) == 0 {
			fmt.Fprintln(c.stdout, "No runs found.")
			return nil
		}
		tw := output.NewTableWriter(c.stdout, []string{"ID", "STATUS", "MESSAGE", "CREATED-AT"}, isTTY)
		for _, run := range runs {
			tw.AddRow(run.ID, string(run.Status), run.Message, run.CreatedAt.Format(time.RFC3339))
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// RunsGetCmd gets a run by ID.
type RunsGetCmd struct {
	ID string `arg:"" help:"ID of the run."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
}

func (c *RunsGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	run, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get run: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toRunJSON(run)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", run.ID)
		tw.AddRow("Status", string(run.Status))
		tw.AddRow("Message", run.Message)
		tw.AddRow("Source", string(run.Source))
		tw.AddRow("Created At", run.CreatedAt.Format(time.RFC3339))
		if run.Workspace != nil {
			tw.AddRow("Workspace ID", run.Workspace.ID)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// RunsCreateCmd creates a new run.
type RunsCreateCmd struct {
	WorkspaceID string `name:"workspace-id" required:"" help:"ID of the workspace."`
	Message     string `help:"Message to associate with the run."`
	AutoApply   bool   `name:"auto-apply" help:"Automatically apply the run if the plan succeeds."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
}

func (c *RunsCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	opts := tfe.RunCreateOptions{
		Workspace: &tfe.Workspace{ID: c.WorkspaceID},
	}
	if c.Message != "" {
		opts.Message = tfe.String(c.Message)
	}
	if c.AutoApply {
		opts.AutoApply = tfe.Bool(c.AutoApply)
	}

	run, err := client.Create(ctx, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create run: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toRunJSON(run)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Run created (ID: %s, Status: %s).\n", run.ID, run.Status)
	}

	return nil
}

// RunsApplyCmd applies a run.
type RunsApplyCmd struct {
	ID      string `arg:"" help:"ID of the run to apply."`
	Comment string `help:"Comment to associate with the apply action."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *RunsApplyCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm action unless --force
	if !force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Apply run %q? This will make changes to your infrastructure.", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting apply.")
			return nil
		}
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	opts := tfe.RunApplyOptions{}
	if c.Comment != "" {
		opts.Comment = tfe.String(c.Comment)
	}

	err = client.Apply(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to apply run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to apply run: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 202); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Run %q apply initiated.\n", c.ID)
	}

	return nil
}

// RunsDiscardCmd discards a run.
type RunsDiscardCmd struct {
	ID      string `arg:"" help:"ID of the run to discard."`
	Comment string `help:"Comment to associate with the discard action."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *RunsDiscardCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm action unless --force
	if !force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Discard run %q?", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting discard.")
			return nil
		}
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	opts := tfe.RunDiscardOptions{}
	if c.Comment != "" {
		opts.Comment = tfe.String(c.Comment)
	}

	err = client.Discard(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to discard run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to discard run: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 202); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Run %q discard initiated.\n", c.ID)
	}

	return nil
}

// RunsCancelCmd cancels a run.
type RunsCancelCmd struct {
	ID      string `arg:"" help:"ID of the run to cancel."`
	Comment string `help:"Comment to associate with the cancel action."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *RunsCancelCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm action unless --force
	if !force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Cancel run %q?", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting cancel.")
			return nil
		}
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	opts := tfe.RunCancelOptions{}
	if c.Comment != "" {
		opts.Comment = tfe.String(c.Comment)
	}

	err = client.Cancel(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to cancel run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to cancel run: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 202); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Run %q cancel initiated.\n", c.ID)
	}

	return nil
}

// RunsForceCancelCmd force-cancels a run.
type RunsForceCancelCmd struct {
	ID      string `arg:"" help:"ID of the run to force-cancel."`
	Comment string `help:"Comment to associate with the force-cancel action."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory runsClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *RunsForceCancelCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultRunsClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm action unless --force
	if !force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Force-cancel run %q? This may leave infrastructure in an inconsistent state.", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting force-cancel.")
			return nil
		}
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	opts := tfe.RunForceCancelOptions{}
	if c.Comment != "" {
		opts.Comment = tfe.String(c.Comment)
	}

	err = client.ForceCancel(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to force-cancel run: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to force-cancel run: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 202); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Run %q force-cancel initiated.\n", c.ID)
	}

	return nil
}
