package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// PlansCmd groups all plans subcommands.
type PlansCmd struct {
	Get           PlansGetCmd           `cmd:"" help:"Get a plan by ID."`
	JSONOutput    PlansJSONOutputCmd    `cmd:"" name:"json-output" help:"Download the JSON execution plan."`
	SanitizedPlan PlansSanitizedPlanCmd `cmd:"" name:"sanitized-plan" help:"Download the sanitized plan (HYOK feature)."`
}

// planJSON is a JSON-serializable representation of a plan.
type planJSON struct {
	ID                   string `json:"id"`
	Status               string `json:"status"`
	HasChanges           bool   `json:"has_changes"`
	ResourceAdditions    int    `json:"resource_additions"`
	ResourceChanges      int    `json:"resource_changes"`
	ResourceDestructions int    `json:"resource_destructions"`
	ResourceImports      int    `json:"resource_imports"`
	LogReadURL           string `json:"log_read_url,omitempty"`
}

// toPlanJSON converts a tfe.Plan to a JSON-serializable form.
func toPlanJSON(plan *tfe.Plan) *planJSON {
	return &planJSON{
		ID:                   plan.ID,
		Status:               string(plan.Status),
		HasChanges:           plan.HasChanges,
		ResourceAdditions:    plan.ResourceAdditions,
		ResourceChanges:      plan.ResourceChanges,
		ResourceDestructions: plan.ResourceDestructions,
		ResourceImports:      plan.ResourceImports,
		LogReadURL:           plan.LogReadURL,
	}
}

// plansClient abstracts the TFC plans API for testing.
type plansClient interface {
	Read(ctx context.Context, planID string) (*tfe.Plan, error)
	ReadJSONOutput(ctx context.Context, planID string) ([]byte, error)
}

// plansClientFactory creates a plansClient from config.
type plansClientFactory func(cfg tfcapi.ClientConfig) (plansClient, error)

// realPlansClient wraps a tfe.Client to implement plansClient.
type realPlansClient struct {
	client *tfe.Client
}

func (c *realPlansClient) Read(ctx context.Context, planID string) (*tfe.Plan, error) {
	return c.client.Plans.Read(ctx, planID)
}

func (c *realPlansClient) ReadJSONOutput(ctx context.Context, planID string) ([]byte, error) {
	return c.client.Plans.ReadJSONOutput(ctx, planID)
}

// defaultPlansClientFactory creates a real TFC client that satisfies plansClient.
func defaultPlansClientFactory(cfg tfcapi.ClientConfig) (plansClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realPlansClient{client: client}, nil
}

// resolvePlansClientConfig resolves settings and token for API calls.
func resolvePlansClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, error) {
	settings, err := config.Load(baseDir)
	if err != nil {
		return tfcapi.ClientConfig{}, err
	}

	contextName := cli.Context
	if contextName == "" {
		contextName = settings.CurrentContext
	}
	ctx, exists := settings.Contexts[contextName]
	if !exists {
		return tfcapi.ClientConfig{}, fmt.Errorf("context %q not found", contextName)
	}

	resolved := ctx.WithDefaults()
	if cli.Address != "" {
		resolved.Address = cli.Address
	}

	if tokenResolver == nil {
		tokenResolver = auth.NewTokenResolver()
	}
	tokenResult, err := tokenResolver.ResolveToken(resolved.Address)
	if err != nil {
		return tfcapi.ClientConfig{}, err
	}

	return tfcapi.ClientConfig{
		Address: resolved.Address,
		Token:   tokenResult.Token,
	}, nil
}

// PlansGetCmd gets a plan by ID.
type PlansGetCmd struct {
	ID string `arg:"" help:"ID of the plan."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory plansClientFactory
}

func (c *PlansGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultPlansClientFactory
	}

	cfg, err := resolvePlansClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	plan, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toPlanJSON(plan)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", plan.ID)
		tw.AddRow("Status", string(plan.Status))
		tw.AddRow("Has Changes", fmt.Sprintf("%t", plan.HasChanges))
		tw.AddRow("Additions", fmt.Sprintf("%d", plan.ResourceAdditions))
		tw.AddRow("Changes", fmt.Sprintf("%d", plan.ResourceChanges))
		tw.AddRow("Destructions", fmt.Sprintf("%d", plan.ResourceDestructions))
		tw.AddRow("Imports", fmt.Sprintf("%d", plan.ResourceImports))
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// PlansJSONOutputCmd downloads the JSON execution plan.
type PlansJSONOutputCmd struct {
	ID  string `arg:"" help:"ID of the plan."`
	Out string `help:"Write output to file instead of stdout."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory plansClientFactory
}

func (c *PlansJSONOutputCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultPlansClientFactory
	}

	cfg, err := resolvePlansClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	jsonBytes, err := client.ReadJSONOutput(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan JSON output: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan JSON output: %w", err))
	}

	// Determine output format for meta output
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if c.Out != "" {
		// Write to file
		if err := os.WriteFile(c.Out, jsonBytes, 0o644); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write file: %w", err))
		}

		// Emit meta JSON/table summary
		if format == output.FormatJSON {
			result := map[string]any{
				"meta": map[string]any{
					"written_to": c.Out,
					"bytes":      len(jsonBytes),
				},
			}
			if err := output.WriteJSON(c.stdout, result); err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
			}
		} else {
			fmt.Fprintf(c.stdout, "Plan JSON output written to %s (%d bytes).\n", c.Out, len(jsonBytes))
		}
	} else {
		// Write to stdout
		if _, err := c.stdout.Write(jsonBytes); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// PlansSanitizedPlanCmd downloads the sanitized plan (HYOK feature).
type PlansSanitizedPlanCmd struct {
	ID  string `arg:"" help:"ID of the plan."`
	Out string `help:"Write output to file instead of stdout."`

	// Dependencies for testing
	baseDir        string
	tokenResolver  *auth.TokenResolver
	ttyDetector    output.TTYDetector
	stdout         io.Writer
	clientFactory  plansClientFactory
	httpClient     *http.Client
	downloadClient func(url string) ([]byte, error)
}

func (c *PlansSanitizedPlanCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultPlansClientFactory
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	if c.downloadClient == nil {
		c.downloadClient = c.defaultDownloadClient
	}

	cfg, err := resolvePlansClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	plan, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get plan: %w", err))
	}

	// Get sanitized-plan link from plan.Links
	sanitizedPlanLink, ok := plan.Links["sanitized-plan"].(string)
	if !ok || sanitizedPlanLink == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("sanitized plan not available for this plan (HYOK feature)"))
	}

	// Download from the sanitized plan URL (no Authorization header - redirect already handled)
	sanitizedBytes, err := c.downloadClient(sanitizedPlanLink)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to download sanitized plan: %w", err))
	}

	// Determine output format for meta output
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if c.Out != "" {
		// Write to file
		if err := os.WriteFile(c.Out, sanitizedBytes, 0o644); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write file: %w", err))
		}

		// Emit meta JSON/table summary
		if format == output.FormatJSON {
			result := map[string]any{
				"meta": map[string]any{
					"written_to": c.Out,
					"bytes":      len(sanitizedBytes),
				},
			}
			if err := output.WriteJSON(c.stdout, result); err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
			}
		} else {
			fmt.Fprintf(c.stdout, "Sanitized plan written to %s (%d bytes).\n", c.Out, len(sanitizedBytes))
		}
	} else {
		// Write to stdout
		if _, err := c.stdout.Write(sanitizedBytes); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// defaultDownloadClient downloads content from a URL without Authorization header.
// This is used for downloading from redirect URLs which should not have auth forwarded.
func (c *PlansSanitizedPlanCmd) defaultDownloadClient(url string) ([]byte, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
