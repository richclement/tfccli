package main

import (
	"context"
	"fmt"
	"io"
	"os"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
	"github.com/richclement/tfccli/internal/ui"
)

// ProjectsCmd groups all projects subcommands.
type ProjectsCmd struct {
	List   ProjectsListCmd   `cmd:"" help:"List projects in an organization."`
	Get    ProjectsGetCmd    `cmd:"" help:"Get a project by ID."`
	Create ProjectsCreateCmd `cmd:"" help:"Create a new project."`
	Update ProjectsUpdateCmd `cmd:"" help:"Update a project."`
	Delete ProjectsDeleteCmd `cmd:"" help:"Delete a project."`
}

// projectJSON is a JSON-serializable representation of a project.
// The go-tfe Project type contains jsonapi.NullableAttr fields that are not
// compatible with standard encoding/json.
type projectJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// toProjectJSON converts a tfe.Project to a JSON-serializable form.
func toProjectJSON(p *tfe.Project) *projectJSON {
	return &projectJSON{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
	}
}

// toProjectJSONList converts a slice of tfe.Project to JSON-serializable form.
func toProjectJSONList(projects []*tfe.Project) []*projectJSON {
	result := make([]*projectJSON, len(projects))
	for i, p := range projects {
		result[i] = toProjectJSON(p)
	}
	return result
}

// projectsClient abstracts the TFC projects API for testing.
type projectsClient interface {
	List(ctx context.Context, org string, opts *tfe.ProjectListOptions) ([]*tfe.Project, error)
	Read(ctx context.Context, projectID string) (*tfe.Project, error)
	Create(ctx context.Context, org string, opts tfe.ProjectCreateOptions) (*tfe.Project, error)
	Update(ctx context.Context, projectID string, opts tfe.ProjectUpdateOptions) (*tfe.Project, error)
	Delete(ctx context.Context, projectID string) error
}

// projectsClientFactory creates a projectsClient from config.
type projectsClientFactory func(cfg tfcapi.ClientConfig) (projectsClient, error)

// realProjectsClient wraps a tfe.Client to implement projectsClient with pagination.
type realProjectsClient struct {
	client *tfe.Client
}

func (c *realProjectsClient) List(ctx context.Context, org string, opts *tfe.ProjectListOptions) ([]*tfe.Project, error) {
	return tfcapi.CollectAllProjects(ctx, c.client, org, opts)
}

func (c *realProjectsClient) Read(ctx context.Context, projectID string) (*tfe.Project, error) {
	return c.client.Projects.Read(ctx, projectID)
}

func (c *realProjectsClient) Create(ctx context.Context, org string, opts tfe.ProjectCreateOptions) (*tfe.Project, error) {
	return c.client.Projects.Create(ctx, org, opts)
}

func (c *realProjectsClient) Update(ctx context.Context, projectID string, opts tfe.ProjectUpdateOptions) (*tfe.Project, error) {
	return c.client.Projects.Update(ctx, projectID, opts)
}

func (c *realProjectsClient) Delete(ctx context.Context, projectID string) error {
	return c.client.Projects.Delete(ctx, projectID)
}

// defaultProjectsClientFactory creates a real TFC client that satisfies projectsClient.
func defaultProjectsClientFactory(cfg tfcapi.ClientConfig) (projectsClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realProjectsClient{client: client}, nil
}

// ProjectsListCmd lists projects in an organization.
type ProjectsListCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory projectsClientFactory
}

func (c *ProjectsListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultProjectsClientFactory
	}

	cfg, org, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
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

	ctx := cmdContext(cli)
	projects, err := client.List(ctx, org, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list projects: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list projects: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Wrap in data array for JSON:API-like output
		// Convert to JSON-serializable form since tfe.Project has jsonapi.NullableAttr fields
		result := map[string]any{"data": toProjectJSONList(projects)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"ID", "NAME", "DESCRIPTION"}, isTTY)
		for _, proj := range projects {
			tw.AddRow(proj.ID, proj.Name, proj.Description)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// ProjectsGetCmd gets a project by ID.
type ProjectsGetCmd struct {
	ID string `arg:"" help:"ID of the project."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory projectsClientFactory
}

func (c *ProjectsGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultProjectsClientFactory
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
	proj, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get project: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get project: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Wrap in data for JSON:API-like output
		// Convert to JSON-serializable form since tfe.Project has jsonapi.NullableAttr fields
		result := map[string]any{"data": toProjectJSON(proj)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", proj.ID)
		tw.AddRow("Name", proj.Name)
		tw.AddRow("Description", proj.Description)
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// ProjectsCreateCmd creates a new project.
type ProjectsCreateCmd struct {
	Name        string `required:"" help:"Name of the project."`
	Description string `help:"Description of the project."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory projectsClientFactory
}

func (c *ProjectsCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultProjectsClientFactory
	}

	cfg, org, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
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

	ctx := cmdContext(cli)
	opts := tfe.ProjectCreateOptions{
		Name: c.Name,
	}
	if c.Description != "" {
		opts.Description = &c.Description
	}

	proj, err := client.Create(ctx, org, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create project: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create project: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Convert to JSON-serializable form since tfe.Project has jsonapi.NullableAttr fields
		result := map[string]any{"data": toProjectJSON(proj)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Project %q created (ID: %s).\n", proj.Name, proj.ID)
	}

	return nil
}

// ProjectsUpdateCmd updates a project.
type ProjectsUpdateCmd struct {
	ID          string `arg:"" help:"ID of the project to update."`
	Name        string `help:"New name for the project."`
	Description string `help:"New description for the project."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory projectsClientFactory
}

func (c *ProjectsUpdateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultProjectsClientFactory
	}

	// Validate at least one field is being updated
	if c.Name == "" && c.Description == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("at least one of --name or --description is required"))
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
	opts := tfe.ProjectUpdateOptions{}
	if c.Name != "" {
		opts.Name = &c.Name
	}
	if c.Description != "" {
		opts.Description = &c.Description
	}

	proj, err := client.Update(ctx, c.ID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to update project: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to update project: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Convert to JSON-serializable form since tfe.Project has jsonapi.NullableAttr fields
		result := map[string]any{"data": toProjectJSON(proj)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Project %q updated.\n", proj.Name)
	}

	return nil
}

// ProjectsDeleteCmd deletes a project.
type ProjectsDeleteCmd struct {
	ID string `arg:"" help:"ID of the project to delete."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory projectsClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *ProjectsDeleteCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultProjectsClientFactory
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
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Delete project %q? This cannot be undone.", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting deletion.")
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
	err = client.Delete(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete project: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete project: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 204); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Project %q deleted.\n", c.ID)
	}

	return nil
}
