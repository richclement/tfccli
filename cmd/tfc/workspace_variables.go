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

// WorkspaceVariablesCmd groups all workspace-variables subcommands.
type WorkspaceVariablesCmd struct {
	List   WorkspaceVariablesListCmd   `cmd:"" help:"List variables for a workspace."`
	Get    WorkspaceVariablesGetCmd    `cmd:"" help:"Get a variable by ID."`
	Create WorkspaceVariablesCreateCmd `cmd:"" help:"Create a new variable."`
	Update WorkspaceVariablesUpdateCmd `cmd:"" help:"Update a variable."`
	Delete WorkspaceVariablesDeleteCmd `cmd:"" help:"Delete a variable."`
}

// variableJSON is a JSON-serializable representation of a variable.
type variableJSON struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive"`
	HCL         bool   `json:"hcl"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// toVariableJSON converts a tfe.Variable to a JSON-serializable form.
func toVariableJSON(v *tfe.Variable) *variableJSON {
	result := &variableJSON{
		ID:          v.ID,
		Key:         v.Key,
		Value:       v.Value,
		Category:    string(v.Category),
		Description: v.Description,
		Sensitive:   v.Sensitive,
		HCL:         v.HCL,
	}
	if v.Workspace != nil {
		result.WorkspaceID = v.Workspace.ID
	}
	return result
}

// toVariableJSONList converts a slice of tfe.Variable to JSON-serializable form.
func toVariableJSONList(variables []*tfe.Variable) []*variableJSON {
	result := make([]*variableJSON, len(variables))
	for i, v := range variables {
		result[i] = toVariableJSON(v)
	}
	return result
}

// variablesClient abstracts the TFC variables API for testing.
type variablesClient interface {
	List(ctx context.Context, workspaceID string, opts *tfe.VariableListOptions) ([]*tfe.Variable, error)
	Read(ctx context.Context, workspaceID, variableID string) (*tfe.Variable, error)
	Create(ctx context.Context, workspaceID string, opts tfe.VariableCreateOptions) (*tfe.Variable, error)
	Update(ctx context.Context, workspaceID, variableID string, opts tfe.VariableUpdateOptions) (*tfe.Variable, error)
	Delete(ctx context.Context, workspaceID, variableID string) error
}

// variablesClientFactory creates a variablesClient from config.
type variablesClientFactory func(cfg tfcapi.ClientConfig) (variablesClient, error)

// realVariablesClient wraps a tfe.Client to implement variablesClient with pagination.
type realVariablesClient struct {
	client *tfe.Client
}

func (c *realVariablesClient) List(ctx context.Context, workspaceID string, opts *tfe.VariableListOptions) ([]*tfe.Variable, error) {
	return tfcapi.CollectAllVariables(ctx, c.client, workspaceID, opts)
}

func (c *realVariablesClient) Read(ctx context.Context, workspaceID, variableID string) (*tfe.Variable, error) {
	return c.client.Variables.Read(ctx, workspaceID, variableID)
}

func (c *realVariablesClient) Create(ctx context.Context, workspaceID string, opts tfe.VariableCreateOptions) (*tfe.Variable, error) {
	return c.client.Variables.Create(ctx, workspaceID, opts)
}

func (c *realVariablesClient) Update(ctx context.Context, workspaceID, variableID string, opts tfe.VariableUpdateOptions) (*tfe.Variable, error) {
	return c.client.Variables.Update(ctx, workspaceID, variableID, opts)
}

func (c *realVariablesClient) Delete(ctx context.Context, workspaceID, variableID string) error {
	return c.client.Variables.Delete(ctx, workspaceID, variableID)
}

// defaultVariablesClientFactory creates a real TFC client that satisfies variablesClient.
func defaultVariablesClientFactory(cfg tfcapi.ClientConfig) (variablesClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realVariablesClient{client: client}, nil
}

// WorkspaceVariablesListCmd lists variables for a workspace.
type WorkspaceVariablesListCmd struct {
	WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory variablesClientFactory
}

func (c *WorkspaceVariablesListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultVariablesClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	variables, err := client.List(ctx, c.WorkspaceID, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list variables: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list variables: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toVariableJSONList(variables)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"ID", "KEY", "CATEGORY", "SENSITIVE", "HCL"}, isTTY)
		for _, v := range variables {
			tw.AddRow(v.ID, v.Key, string(v.Category), fmt.Sprintf("%t", v.Sensitive), fmt.Sprintf("%t", v.HCL))
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// WorkspaceVariablesGetCmd retrieves a single variable by ID.
type WorkspaceVariablesGetCmd struct {
	VariableID  string `arg:"" help:"ID of the variable to retrieve."`
	WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory variablesClientFactory
}

func (c *WorkspaceVariablesGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultVariablesClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	variable, err := client.Read(ctx, c.WorkspaceID, c.VariableID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get variable: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get variable: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toVariableJSON(variable)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", variable.ID)
		tw.AddRow("Key", variable.Key)
		tw.AddRow("Category", string(variable.Category))
		tw.AddRow("Sensitive", fmt.Sprintf("%t", variable.Sensitive))
		tw.AddRow("HCL", fmt.Sprintf("%t", variable.HCL))
		if variable.Description != "" {
			tw.AddRow("Description", variable.Description)
		}
		if variable.Workspace != nil {
			tw.AddRow("Workspace ID", variable.Workspace.ID)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// WorkspaceVariablesCreateCmd creates a new variable.
type WorkspaceVariablesCreateCmd struct {
	WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`
	Key         string `required:"" help:"Key name of the variable."`
	Value       string `required:"" help:"Value of the variable."`
	Category    string `required:"" enum:"env,terraform" help:"Category: env or terraform."`
	Description string `help:"Description of the variable."`
	Sensitive   bool   `help:"Mark the variable as sensitive."`
	HCL         bool   `help:"Parse the value as HCL."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory variablesClientFactory
}

func (c *WorkspaceVariablesCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultVariablesClientFactory
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	// Map category string to tfe.CategoryType
	var category tfe.CategoryType
	switch c.Category {
	case "env":
		category = tfe.CategoryEnv
	case "terraform":
		category = tfe.CategoryTerraform
	}

	ctx := context.Background()
	opts := tfe.VariableCreateOptions{
		Key:       tfe.String(c.Key),
		Value:     tfe.String(c.Value),
		Category:  tfe.Category(category),
		Sensitive: tfe.Bool(c.Sensitive),
		HCL:       tfe.Bool(c.HCL),
	}
	if c.Description != "" {
		opts.Description = tfe.String(c.Description)
	}

	variable, err := client.Create(ctx, c.WorkspaceID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create variable: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create variable: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toVariableJSON(variable)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Variable %q created (ID: %s).\n", variable.Key, variable.ID)
	}

	return nil
}

// WorkspaceVariablesUpdateCmd updates a variable.
type WorkspaceVariablesUpdateCmd struct {
	VariableID       string `arg:"" help:"ID of the variable to update."`
	WorkspaceID      string `required:"" name:"workspace-id" help:"ID of the workspace."`
	Key              string `help:"New key name of the variable."`
	Value            string `help:"New value of the variable."`
	ClearValue       bool   `name:"clear-value" help:"Clear the variable value."`
	Description      string `help:"New description of the variable."`
	ClearDescription bool   `name:"clear-description" help:"Clear the variable description."`
	Sensitive        *bool  `help:"Mark the variable as sensitive."`
	HCL              *bool  `help:"Parse the value as HCL."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory variablesClientFactory
}

func (c *WorkspaceVariablesUpdateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultVariablesClientFactory
	}

	// Validate --value and --clear-value are mutually exclusive
	if c.Value != "" && c.ClearValue {
		return internalcmd.NewRuntimeError(fmt.Errorf("--value and --clear-value are mutually exclusive"))
	}

	// Validate --description and --clear-description are mutually exclusive
	if c.Description != "" && c.ClearDescription {
		return internalcmd.NewRuntimeError(fmt.Errorf("--description and --clear-description are mutually exclusive"))
	}

	// Validate at least one field is being updated
	if c.Key == "" && c.Value == "" && !c.ClearValue && c.Description == "" && !c.ClearDescription && c.Sensitive == nil && c.HCL == nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("at least one of --key, --value, --clear-value, --description, --clear-description, --sensitive, or --hcl is required"))
	}

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	opts := tfe.VariableUpdateOptions{}
	if c.Key != "" {
		opts.Key = tfe.String(c.Key)
	}
	if c.Value != "" {
		opts.Value = tfe.String(c.Value)
	} else if c.ClearValue {
		opts.Value = tfe.String("")
	}
	if c.Description != "" {
		opts.Description = tfe.String(c.Description)
	} else if c.ClearDescription {
		opts.Description = tfe.String("")
	}
	if c.Sensitive != nil {
		opts.Sensitive = c.Sensitive
	}
	if c.HCL != nil {
		opts.HCL = c.HCL
	}

	variable, err := client.Update(ctx, c.WorkspaceID, c.VariableID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to update variable: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to update variable: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		result := map[string]any{"data": toVariableJSON(variable)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Variable %q updated.\n", variable.Key)
	}

	return nil
}

// WorkspaceVariablesDeleteCmd deletes a variable.
type WorkspaceVariablesDeleteCmd struct {
	VariableID  string `arg:"" help:"ID of the variable to delete."`
	WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory variablesClientFactory
	prompter      ui.Prompter
	forceFlag     *bool
}

func (c *WorkspaceVariablesDeleteCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultVariablesClientFactory
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
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Delete variable %q? This cannot be undone.", c.VariableID), false)
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

	ctx := context.Background()
	err = client.Delete(ctx, c.WorkspaceID, c.VariableID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete variable: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to delete variable: %w", err))
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
		fmt.Fprintf(c.stdout, "Variable %q deleted.\n", c.VariableID)
	}

	return nil
}
