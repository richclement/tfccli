package main

import (
	"context"
	"fmt"
	"io"
	"os"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
	"github.com/richclement/tfccli/internal/ui"
)

// WorkspacesCmd groups all workspaces subcommands.
type WorkspacesCmd struct {
	List   WorkspacesListCmd   `cmd:"" help:"List workspaces in an organization."`
	Get    WorkspacesGetCmd    `cmd:"" help:"Get a workspace by ID."`
	Create WorkspacesCreateCmd `cmd:"" help:"Create a new workspace."`
	Update WorkspacesUpdateCmd `cmd:"" help:"Update a workspace."`
	Delete WorkspacesDeleteCmd `cmd:"" help:"Delete a workspace."`
}

// workspaceJSON is a JSON-serializable representation of a workspace.
type workspaceJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	ExecutionMode string `json:"execution_mode"`
	ProjectID     string `json:"project_id,omitempty"`
}

// toWorkspaceJSON converts a tfe.Workspace to a JSON-serializable form.
func toWorkspaceJSON(ws *tfe.Workspace) *workspaceJSON {
	result := &workspaceJSON{
		ID:            ws.ID,
		Name:          ws.Name,
		Description:   ws.Description,
		ExecutionMode: string(ws.ExecutionMode),
	}
	if ws.Project != nil {
		result.ProjectID = ws.Project.ID
	}
	return result
}

// toWorkspaceJSONList converts a slice of tfe.Workspace to JSON-serializable form.
func toWorkspaceJSONList(workspaces []*tfe.Workspace) []*workspaceJSON {
	result := make([]*workspaceJSON, len(workspaces))
	for i, ws := range workspaces {
		result[i] = toWorkspaceJSON(ws)
	}
	return result
}

// workspacesClient abstracts the TFC workspaces API for testing.
type workspacesClient interface {
	List(ctx context.Context, org string, opts *tfe.WorkspaceListOptions) ([]*tfe.Workspace, error)
	ReadByID(ctx context.Context, workspaceID string) (*tfe.Workspace, error)
	Create(ctx context.Context, org string, opts tfe.WorkspaceCreateOptions) (*tfe.Workspace, error)
	UpdateByID(ctx context.Context, workspaceID string, opts tfe.WorkspaceUpdateOptions) (*tfe.Workspace, error)
	DeleteByID(ctx context.Context, workspaceID string) error
}

// workspacesClientFactory creates a workspacesClient from config.
type workspacesClientFactory func(cfg tfcapi.ClientConfig) (workspacesClient, error)

// realWorkspacesClient wraps a tfe.Client to implement workspacesClient with pagination.
type realWorkspacesClient struct {
	client *tfe.Client
}

func (c *realWorkspacesClient) List(ctx context.Context, org string, opts *tfe.WorkspaceListOptions) ([]*tfe.Workspace, error) {
	return tfcapi.CollectAllWorkspaces(ctx, c.client, org, opts)
}

func (c *realWorkspacesClient) ReadByID(ctx context.Context, workspaceID string) (*tfe.Workspace, error) {
	return c.client.Workspaces.ReadByID(ctx, workspaceID)
}

func (c *realWorkspacesClient) Create(ctx context.Context, org string, opts tfe.WorkspaceCreateOptions) (*tfe.Workspace, error) {
	return c.client.Workspaces.Create(ctx, org, opts)
}

func (c *realWorkspacesClient) UpdateByID(ctx context.Context, workspaceID string, opts tfe.WorkspaceUpdateOptions) (*tfe.Workspace, error) {
	return c.client.Workspaces.UpdateByID(ctx, workspaceID, opts)
}

func (c *realWorkspacesClient) DeleteByID(ctx context.Context, workspaceID string) error {
	return c.client.Workspaces.DeleteByID(ctx, workspaceID)
}

// defaultWorkspacesClientFactory creates a real TFC client that satisfies workspacesClient.
func defaultWorkspacesClientFactory(cfg tfcapi.ClientConfig) (workspacesClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realWorkspacesClient{client: client}, nil
}

// resolveWorkspacesClientConfig resolves settings and token for API calls, including org resolution.
func resolveWorkspacesClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, string, error) {
	settings, err := config.Load(baseDir)
	if err != nil {
		return tfcapi.ClientConfig{}, "", err
	}

	contextName := cli.Context
	if contextName == "" {
		contextName = settings.CurrentContext
	}
	ctx, exists := settings.Contexts[contextName]
	if !exists {
		return tfcapi.ClientConfig{}, "", fmt.Errorf("context %q not found", contextName)
	}

	resolved := ctx.WithDefaults()
	if cli.Address != "" {
		resolved.Address = cli.Address
	}

	// Resolve organization: CLI flag takes precedence over context default
	org := cli.Org
	if org == "" {
		org = resolved.DefaultOrg
	}

	if tokenResolver == nil {
		tokenResolver = auth.NewTokenResolver()
	}
	tokenResult, err := tokenResolver.ResolveToken(resolved.Address)
	if err != nil {
		return tfcapi.ClientConfig{}, "", err
	}

	return tfcapi.ClientConfig{
		Address: resolved.Address,
		Token:   tokenResult.Token,
	}, org, nil
}

// WorkspacesListCmd lists workspaces in an organization.
type WorkspacesListCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspacesClientFactory
}

func (c *WorkspacesListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspacesClientFactory
	}

	cfg, org, err := resolveWorkspacesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	// Validate org is available
	if org == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("organization is required; use --org flag or set default_org in context"))
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	workspaces, err := client.List(ctx, org, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list workspaces: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list workspaces: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toWorkspaceJSONList(workspaces)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"ID", "NAME", "EXECUTION-MODE", "PROJECT-ID"}, isTTY)
		for _, ws := range workspaces {
			projectID := ""
			if ws.Project != nil {
				projectID = ws.Project.ID
			}
			tw.AddRow(ws.ID, ws.Name, string(ws.ExecutionMode), projectID)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// WorkspacesGetCmd gets a workspace by ID.
type WorkspacesGetCmd struct {
	ID string `arg:"" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspacesClientFactory
}

func (c *WorkspacesGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspacesClientFactory
	}

	cfg, _, err := resolveWorkspacesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	ws, err := client.ReadByID(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get workspace: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get workspace: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toWorkspaceJSON(ws)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", ws.ID)
		tw.AddRow("Name", ws.Name)
		tw.AddRow("Description", ws.Description)
		tw.AddRow("Execution Mode", string(ws.ExecutionMode))
		if ws.Project != nil {
			tw.AddRow("Project ID", ws.Project.ID)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// WorkspacesCreateCmd creates a new workspace.
type WorkspacesCreateCmd struct {
	Name        string `required:"" help:"Name of the workspace."`
	Description string `help:"Description of the workspace."`
	ProjectID   string `name:"project-id" help:"ID of the project to create the workspace in."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspacesClientFactory
}

func (c *WorkspacesCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspacesClientFactory
	}

	cfg, org, err := resolveWorkspacesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	// Validate org is available
	if org == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("organization is required; use --org flag or set default_org in context"))
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	opts := tfe.WorkspaceCreateOptions{
		Name: tfe.String(c.Name),
	}
	if c.Description != "" {
		opts.Description = tfe.String(c.Description)
	}
	if c.ProjectID != "" {
		opts.Project = &tfe.Project{ID: c.ProjectID}
	}

	ws, err := client.Create(ctx, org, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create workspace: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create workspace: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toWorkspaceJSON(ws)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Workspace %q created (ID: %s).\n", ws.Name, ws.ID)
	}

	return nil
}

// WorkspacesUpdateCmd updates a workspace.
type WorkspacesUpdateCmd struct {
	ID          string `arg:"" help:"ID of the workspace to update."`
	Name        string `help:"New name for the workspace."`
	Description string `help:"New description for the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspacesClientFactory
}

func (c *WorkspacesUpdateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspacesClientFactory
	}

	cfg, _, err := resolveWorkspacesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	opts := tfe.WorkspaceUpdateOptions{}
	if c.Name != "" {
		opts.Name = tfe.String(c.Name)
	}
	if c.Description != "" {
		opts.Description = tfe.String(c.Description)
	}

	ws, err := client.UpdateByID(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to update workspace: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to update workspace: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toWorkspaceJSON(ws)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Workspace %q updated.\n", ws.Name)
	}

	return nil
}

// WorkspacesDeleteCmd deletes a workspace.
type WorkspacesDeleteCmd struct {
	ID string `arg:"" help:"ID of the workspace to delete."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspacesClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *WorkspacesDeleteCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspacesClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Get force flag from CLI or injected value
	force := cli.Force
	if c.forceFlag != nil {
		force = *c.forceFlag
	}

	// Confirm deletion unless --force
	if !force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Delete workspace %q? This cannot be undone.", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting deletion.")
			return nil
		}
	}

	cfg, _, err := resolveWorkspacesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	err = client.DeleteByID(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete workspace: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete workspace: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 204); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Workspace %q deleted.\n", c.ID)
	}

	return nil
}
