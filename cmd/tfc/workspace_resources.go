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
)

// WorkspaceResourcesCmd groups all workspace-resources subcommands.
type WorkspaceResourcesCmd struct {
	List WorkspaceResourcesListCmd `cmd:"" help:"List resources in a workspace."`
}

// workspaceResourceJSON is a JSON-serializable representation of a workspace resource.
type workspaceResourceJSON struct {
	ID           string `json:"id"`
	Address      string `json:"address"`
	Name         string `json:"name"`
	Module       string `json:"module,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ProviderType string `json:"provider_type,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// toWorkspaceResourceJSON converts a tfe.WorkspaceResource to a JSON-serializable form.
func toWorkspaceResourceJSON(r *tfe.WorkspaceResource) *workspaceResourceJSON {
	return &workspaceResourceJSON{
		ID:           r.ID,
		Address:      r.Address,
		Name:         r.Name,
		Module:       r.Module,
		Provider:     r.Provider,
		ProviderType: r.ProviderType,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

// toWorkspaceResourceJSONList converts a slice of tfe.WorkspaceResource to JSON-serializable form.
func toWorkspaceResourceJSONList(resources []*tfe.WorkspaceResource) []*workspaceResourceJSON {
	result := make([]*workspaceResourceJSON, len(resources))
	for i, r := range resources {
		result[i] = toWorkspaceResourceJSON(r)
	}
	return result
}

// workspaceResourcesClient abstracts the TFC workspace resources API for testing.
type workspaceResourcesClient interface {
	List(ctx context.Context, workspaceID string, opts *tfe.WorkspaceResourceListOptions) ([]*tfe.WorkspaceResource, error)
}

// workspaceResourcesClientFactory creates a workspaceResourcesClient from config.
type workspaceResourcesClientFactory func(cfg tfcapi.ClientConfig) (workspaceResourcesClient, error)

// realWorkspaceResourcesClient wraps a tfe.Client to implement workspaceResourcesClient with pagination.
type realWorkspaceResourcesClient struct {
	client *tfe.Client
}

func (c *realWorkspaceResourcesClient) List(ctx context.Context, workspaceID string, opts *tfe.WorkspaceResourceListOptions) ([]*tfe.WorkspaceResource, error) {
	return tfcapi.CollectAllWorkspaceResources(ctx, c.client, workspaceID, opts)
}

// defaultWorkspaceResourcesClientFactory creates a real TFC client that satisfies workspaceResourcesClient.
func defaultWorkspaceResourcesClientFactory(cfg tfcapi.ClientConfig) (workspaceResourcesClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realWorkspaceResourcesClient{client: client}, nil
}

// WorkspaceResourcesListCmd lists resources in a workspace.
type WorkspaceResourcesListCmd struct {
	WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory workspaceResourcesClientFactory
}

func (c *WorkspaceResourcesListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultWorkspaceResourcesClientFactory
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
	resources, err := client.List(ctx, c.WorkspaceID, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list workspace resources: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list workspace resources: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toWorkspaceResourceJSONList(resources)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"ID", "RESOURCE-TYPE", "NAME", "PROVIDER"}, isTTY)
		for _, r := range resources {
			tw.AddRow(r.ID, r.ProviderType, r.Name, r.Provider)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}
