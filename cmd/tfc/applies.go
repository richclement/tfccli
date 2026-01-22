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
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// AppliesCmd groups all applies subcommands.
type AppliesCmd struct {
	Get          AppliesGetCmd          `cmd:"" help:"Get an apply by ID."`
	ErroredState AppliesErroredStateCmd `cmd:"" name:"errored-state" help:"Download the errored state from a failed apply."`
}

// applyJSON is a JSON-serializable representation of an apply.
type applyJSON struct {
	ID                   string `json:"id"`
	Status               string `json:"status"`
	ResourceAdditions    int    `json:"resource_additions"`
	ResourceChanges      int    `json:"resource_changes"`
	ResourceDestructions int    `json:"resource_destructions"`
	ResourceImports      int    `json:"resource_imports"`
	LogReadURL           string `json:"log_read_url,omitempty"`
}

// toApplyJSON converts a tfe.Apply to a JSON-serializable form.
func toApplyJSON(apply *tfe.Apply) *applyJSON {
	return &applyJSON{
		ID:                   apply.ID,
		Status:               string(apply.Status),
		ResourceAdditions:    apply.ResourceAdditions,
		ResourceChanges:      apply.ResourceChanges,
		ResourceDestructions: apply.ResourceDestructions,
		ResourceImports:      apply.ResourceImports,
		LogReadURL:           apply.LogReadURL,
	}
}

// appliesClient abstracts the TFC applies API for testing.
type appliesClient interface {
	Read(ctx context.Context, applyID string) (*tfe.Apply, error)
	// GetErroredStateURL returns the redirect URL for downloading errored state.
	// Returns empty string if errored state is not available.
	GetErroredStateURL(ctx context.Context, applyID string) (string, error)
}

// appliesClientFactory creates an appliesClient from config.
type appliesClientFactory func(cfg tfcapi.ClientConfig) (appliesClient, error)

// realAppliesClient wraps a tfe.Client to implement appliesClient.
type realAppliesClient struct {
	client *tfe.Client
	cfg    tfcapi.ClientConfig
}

func (c *realAppliesClient) Read(ctx context.Context, applyID string) (*tfe.Apply, error) {
	return c.client.Applies.Read(ctx, applyID)
}

// GetErroredStateURL fetches the errored state redirect URL.
// The TFC API returns a 307 redirect with a Location header.
// We use a custom HTTP client that doesn't follow redirects to capture the URL.
func (c *realAppliesClient) GetErroredStateURL(ctx context.Context, applyID string) (string, error) {
	// Build the URL for the errored state endpoint
	baseURL := tfcapi.APIBaseURL(c.cfg.Address)
	url := fmt.Sprintf("%s/applies/%s/errored-state", baseURL, applyID)

	// Create HTTP client that doesn't follow redirects
	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("errored state not available (apply not found, no errored state uploaded, or unauthorized)")
	}

	if resp.StatusCode == http.StatusTemporaryRedirect {
		location := resp.Header.Get("Location")
		if location == "" {
			return "", fmt.Errorf("redirect response missing Location header")
		}
		return location, nil
	}

	// Read body for error message
	body, _ := io.ReadAll(resp.Body)
	return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
}

// defaultAppliesClientFactory creates a real TFC client that satisfies appliesClient.
func defaultAppliesClientFactory(cfg tfcapi.ClientConfig) (appliesClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realAppliesClient{client: client, cfg: cfg}, nil
}

// AppliesGetCmd gets an apply by ID.
type AppliesGetCmd struct {
	ID string `arg:"" help:"ID of the apply."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory appliesClientFactory
}

func (c *AppliesGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultAppliesClientFactory
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
	apply, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get apply: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get apply: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toApplyJSON(apply)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", apply.ID)
		tw.AddRow("Status", string(apply.Status))
		tw.AddRow("Additions", fmt.Sprintf("%d", apply.ResourceAdditions))
		tw.AddRow("Changes", fmt.Sprintf("%d", apply.ResourceChanges))
		tw.AddRow("Destructions", fmt.Sprintf("%d", apply.ResourceDestructions))
		tw.AddRow("Imports", fmt.Sprintf("%d", apply.ResourceImports))
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// AppliesErroredStateCmd downloads the errored state from a failed apply.
type AppliesErroredStateCmd struct {
	ID  string `arg:"" help:"ID of the apply."`
	Out string `help:"Write output to file instead of stdout."`

	// Dependencies for testing
	baseDir        string
	tokenResolver  *auth.TokenResolver
	ttyDetector    output.TTYDetector
	stdout         io.Writer
	clientFactory  appliesClientFactory
	downloadClient func(url string) ([]byte, error)
}

func (c *AppliesErroredStateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultAppliesClientFactory
	}
	if c.downloadClient == nil {
		c.downloadClient = c.defaultDownloadClient
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

	// Get the redirect URL for errored state
	erroredStateURL, err := client.GetErroredStateURL(ctx, c.ID)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get errored state URL: %w", err))
	}

	// Download from the errored state URL (no Authorization header - redirect already provided signed URL)
	stateBytes, err := c.downloadClient(erroredStateURL)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to download errored state: %w", err))
	}

	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if c.Out != "" {
		// Write to file
		if err := os.WriteFile(c.Out, stateBytes, 0o644); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write file: %w", err))
		}

		// Emit meta JSON/table summary
		if format == output.FormatJSON {
			result := map[string]any{
				"meta": map[string]any{
					"written_to": c.Out,
					"bytes":      len(stateBytes),
				},
			}
			if err := output.WriteJSON(c.stdout, result); err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
			}
		} else {
			fmt.Fprintf(c.stdout, "Errored state written to %s (%d bytes).\n", c.Out, len(stateBytes))
		}
	} else {
		// Write to stdout
		if _, err := c.stdout.Write(stateBytes); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// defaultDownloadClient downloads content from a URL without Authorization header.
// This is used for downloading from redirect URLs which should not have auth forwarded.
func (c *AppliesErroredStateCmd) defaultDownloadClient(url string) ([]byte, error) {
	httpClient := &http.Client{}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
