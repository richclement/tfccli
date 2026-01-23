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

// OrganizationsCmd groups all organizations subcommands.
type OrganizationsCmd struct {
	List   OrganizationsListCmd   `cmd:"" help:"List all organizations."`
	Get    OrganizationsGetCmd    `cmd:"" help:"Get an organization by name."`
	Create OrganizationsCreateCmd `cmd:"" help:"Create a new organization."`
	Update OrganizationsUpdateCmd `cmd:"" help:"Update an organization."`
	Delete OrganizationsDeleteCmd `cmd:"" help:"Delete an organization."`
}

// orgsClient abstracts the TFC organizations API for testing.
type orgsClient interface {
	List(ctx context.Context, opts *tfe.OrganizationListOptions) ([]*tfe.Organization, error)
	Read(ctx context.Context, name string) (*tfe.Organization, error)
	Create(ctx context.Context, opts tfe.OrganizationCreateOptions) (*tfe.Organization, error)
	Update(ctx context.Context, name string, opts tfe.OrganizationUpdateOptions) (*tfe.Organization, error)
	Delete(ctx context.Context, name string) error
}

// orgsClientFactory creates an orgsClient from config.
type orgsClientFactory func(cfg tfcapi.ClientConfig) (orgsClient, error)

// realOrgsClient wraps a tfe.Client to implement orgsClient with pagination.
type realOrgsClient struct {
	client *tfe.Client
}

func (c *realOrgsClient) List(ctx context.Context, opts *tfe.OrganizationListOptions) ([]*tfe.Organization, error) {
	return tfcapi.CollectAllOrganizations(ctx, c.client, opts)
}

func (c *realOrgsClient) Read(ctx context.Context, name string) (*tfe.Organization, error) {
	return c.client.Organizations.Read(ctx, name)
}

func (c *realOrgsClient) Create(ctx context.Context, opts tfe.OrganizationCreateOptions) (*tfe.Organization, error) {
	return c.client.Organizations.Create(ctx, opts)
}

func (c *realOrgsClient) Update(ctx context.Context, name string, opts tfe.OrganizationUpdateOptions) (*tfe.Organization, error) {
	return c.client.Organizations.Update(ctx, name, opts)
}

func (c *realOrgsClient) Delete(ctx context.Context, name string) error {
	return c.client.Organizations.Delete(ctx, name)
}

// defaultOrgsClientFactory creates a real TFC client that satisfies orgsClient.
func defaultOrgsClientFactory(cfg tfcapi.ClientConfig) (orgsClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realOrgsClient{client: client}, nil
}

// OrganizationsListCmd lists all organizations.
type OrganizationsListCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory orgsClientFactory
}

func (c *OrganizationsListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultOrgsClientFactory
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
	orgs, err := client.List(ctx, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Wrap in data array for JSON:API-like output
		result := map[string]any{"data": orgs}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		if len(orgs) == 0 {
			fmt.Fprintln(c.stdout, "No organizations found.")
			return nil
		}
		tw := output.NewTableWriter(c.stdout, []string{"NAME", "EMAIL", "EXTERNAL-ID"}, isTTY)
		for _, org := range orgs {
			// go-tfe returns empty strings for optional fields (Email, ExternalID), not nil.
			tw.AddRow(org.Name, org.Email, org.ExternalID)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// OrganizationsGetCmd gets an organization by name.
type OrganizationsGetCmd struct {
	Name string `arg:"" help:"Name of the organization."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory orgsClientFactory
}

func (c *OrganizationsGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultOrgsClientFactory
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
	org, err := client.Read(ctx, c.Name)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get organization: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get organization: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		// Wrap in data for JSON:API-like output
		result := map[string]any{"data": org}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("Name", org.Name)
		tw.AddRow("Email", org.Email)
		tw.AddRow("External ID", org.ExternalID)
		tw.AddRow("Created At", org.CreatedAt.String())
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// OrganizationsCreateCmd creates a new organization.
type OrganizationsCreateCmd struct {
	Name  string `required:"" help:"Name of the organization."`
	Email string `required:"" help:"Admin email for the organization."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory orgsClientFactory
}

func (c *OrganizationsCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultOrgsClientFactory
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
	opts := tfe.OrganizationCreateOptions{
		Name:  tfe.String(c.Name),
		Email: tfe.String(c.Email),
	}
	org, err := client.Create(ctx, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create organization: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create organization: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": org}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Organization %q created.\n", org.Name)
	}

	return nil
}

// OrganizationsUpdateCmd updates an organization.
type OrganizationsUpdateCmd struct {
	Name  string `arg:"" help:"Name of the organization to update."`
	Email string `help:"New admin email for the organization."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory orgsClientFactory
}

func (c *OrganizationsUpdateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultOrgsClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	// Validate at least one field is being updated
	if c.Email == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("nothing to update: specify --email"))
	}

	ctx := cmdContext(cli)
	opts := tfe.OrganizationUpdateOptions{
		Email: tfe.String(c.Email),
	}

	org, err := client.Update(ctx, c.Name, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to update organization: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to update organization: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": org}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Organization %q updated.\n", org.Name)
	}

	return nil
}

// OrganizationsDeleteCmd deletes an organization.
type OrganizationsDeleteCmd struct {
	Name string `arg:"" help:"Name of the organization to delete."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory orgsClientFactory
	prompter      ui.Prompter
}

func (c *OrganizationsDeleteCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultOrgsClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Confirm deletion unless --force
	if !cli.Force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Delete organization %q? This cannot be undone.", c.Name), false)
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
	err = client.Delete(ctx, c.Name)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete organization: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete organization: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteEmptySuccess(c.stdout, 204); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Organization %q deleted.\n", c.Name)
	}

	return nil
}
