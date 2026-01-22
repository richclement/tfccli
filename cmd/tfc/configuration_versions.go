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
	"github.com/richclement/tfccli/internal/ui"
)

// ConfigurationVersionsCmd groups all configuration-versions subcommands.
type ConfigurationVersionsCmd struct {
	List     CVListCmd     `cmd:"" help:"List configuration versions for a workspace."`
	Get      CVGetCmd      `cmd:"" help:"Get a configuration version by ID."`
	Create   CVCreateCmd   `cmd:"" help:"Create a new configuration version."`
	Upload   CVUploadCmd   `cmd:"" help:"Upload configuration content to a configuration version."`
	Download CVDownloadCmd `cmd:"" help:"Download configuration content from a configuration version."`
	Archive  CVArchiveCmd  `cmd:"" help:"Archive a configuration version."`
}

// cvJSON is a JSON-serializable representation of a configuration version.
type cvJSON struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Source        string `json:"source,omitempty"`
	AutoQueueRuns bool   `json:"auto_queue_runs"`
	Speculative   bool   `json:"speculative"`
	ErrorMessage  string `json:"error_message,omitempty"`
	UploadURL     string `json:"upload_url,omitempty"`
}

// toCVJSON converts a tfe.ConfigurationVersion to a JSON-serializable form.
func toCVJSON(cv *tfe.ConfigurationVersion) *cvJSON {
	return &cvJSON{
		ID:            cv.ID,
		Status:        string(cv.Status),
		Source:        string(cv.Source),
		AutoQueueRuns: cv.AutoQueueRuns,
		Speculative:   cv.Speculative,
		ErrorMessage:  cv.ErrorMessage,
		UploadURL:     cv.UploadURL,
	}
}

// cvClient abstracts the TFC configuration versions API for testing.
type cvClient interface {
	List(ctx context.Context, workspaceID string, opts *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error)
	Read(ctx context.Context, cvID string) (*tfe.ConfigurationVersion, error)
	Create(ctx context.Context, workspaceID string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error)
	Download(ctx context.Context, cvID string) ([]byte, error)
	Archive(ctx context.Context, cvID string) error
}

// cvClientFactory creates a cvClient from config.
type cvClientFactory func(cfg tfcapi.ClientConfig) (cvClient, error)

// realCVClient wraps a tfe.Client to implement cvClient.
type realCVClient struct {
	client *tfe.Client
}

func (c *realCVClient) List(ctx context.Context, workspaceID string, opts *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
	return tfcapi.CollectAllConfigurationVersions(ctx, c.client, workspaceID, opts)
}

func (c *realCVClient) Read(ctx context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
	return c.client.ConfigurationVersions.Read(ctx, cvID)
}

func (c *realCVClient) Create(ctx context.Context, workspaceID string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
	return c.client.ConfigurationVersions.Create(ctx, workspaceID, opts)
}

func (c *realCVClient) Download(ctx context.Context, cvID string) ([]byte, error) {
	return c.client.ConfigurationVersions.Download(ctx, cvID)
}

func (c *realCVClient) Archive(ctx context.Context, cvID string) error {
	return c.client.ConfigurationVersions.Archive(ctx, cvID)
}

// defaultCVClientFactory creates a real TFC client that satisfies cvClient.
func defaultCVClientFactory(cfg tfcapi.ClientConfig) (cvClient, error) {
	client, err := tfcapi.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &realCVClient{client: client}, nil
}

// resolveCVClientConfig resolves settings and token for API calls.
func resolveCVClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, error) {
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

// CVListCmd lists configuration versions for a workspace.
type CVListCmd struct {
	WorkspaceID string `name:"workspace-id" required:"" help:"ID of the workspace."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
}

func (c *CVListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	cvs, err := client.List(ctx, c.WorkspaceID, nil)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list configuration versions: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list configuration versions: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		jsonCVs := make([]*cvJSON, len(cvs))
		for i, cv := range cvs {
			jsonCVs[i] = toCVJSON(cv)
		}
		result := map[string]any{"data": jsonCVs}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"ID", "STATUS", "SOURCE", "AUTO-QUEUE-RUNS", "SPECULATIVE"}, isTTY)
		for _, cv := range cvs {
			tw.AddRow(cv.ID, string(cv.Status), string(cv.Source), fmt.Sprintf("%t", cv.AutoQueueRuns), fmt.Sprintf("%t", cv.Speculative))
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// CVGetCmd gets a configuration version by ID.
type CVGetCmd struct {
	ID string `arg:"" help:"ID of the configuration version."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
}

func (c *CVGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	cv, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get configuration version: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get configuration version: %w", err))
	}

	// Determine output format
	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toCVJSON(cv)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", cv.ID)
		tw.AddRow("Status", string(cv.Status))
		tw.AddRow("Source", string(cv.Source))
		tw.AddRow("Auto-Queue-Runs", fmt.Sprintf("%t", cv.AutoQueueRuns))
		tw.AddRow("Speculative", fmt.Sprintf("%t", cv.Speculative))
		if cv.ErrorMessage != "" {
			tw.AddRow("Error Message", cv.ErrorMessage)
		}
		if cv.UploadURL != "" {
			tw.AddRow("Upload URL", cv.UploadURL)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// CVCreateCmd creates a new configuration version.
type CVCreateCmd struct {
	WorkspaceID   string `name:"workspace-id" required:"" help:"ID of the workspace."`
	AutoQueueRuns *bool  `name:"auto-queue-runs" help:"Automatically queue runs when configuration is uploaded."`
	Speculative   bool   `help:"Create a speculative configuration version (for speculative plans)."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
}

func (c *CVCreateCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	opts := tfe.ConfigurationVersionCreateOptions{
		Speculative: tfe.Bool(c.Speculative),
	}
	if c.AutoQueueRuns != nil {
		opts.AutoQueueRuns = c.AutoQueueRuns
	}

	ctx := context.Background()
	cv, err := client.Create(ctx, c.WorkspaceID, opts)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to create configuration version: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create configuration version: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{"data": toCVJSON(cv)}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Configuration version %s created.\n", cv.ID)
		fmt.Fprintf(c.stdout, "Status: %s\n", cv.Status)
		if cv.UploadURL != "" {
			fmt.Fprintf(c.stdout, "Upload URL: %s\n", cv.UploadURL)
		}
	}

	return nil
}

// CVUploadCmd uploads configuration content to a configuration version.
type CVUploadCmd struct {
	ID   string `arg:"" help:"ID of the configuration version."`
	File string `required:"" help:"Path to the tar.gz file to upload."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
	uploadClient  func(uploadURL string, fileContent []byte) error
	fileReader    func(path string) ([]byte, error)
}

func (c *CVUploadCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}
	if c.uploadClient == nil {
		c.uploadClient = defaultUploadClient
	}
	if c.fileReader == nil {
		c.fileReader = os.ReadFile
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	// Get the configuration version to retrieve the upload URL
	ctx := context.Background()
	cv, err := client.Read(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get configuration version: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get configuration version: %w", err))
	}

	if cv.UploadURL == "" {
		return internalcmd.NewRuntimeError(fmt.Errorf("configuration version %s has no upload URL (status: %s)", c.ID, cv.Status))
	}

	// Read the file
	fileContent, err := c.fileReader(c.File)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to read file %s: %w", c.File, err))
	}

	// Upload to the URL without Authorization header
	if err := c.uploadClient(cv.UploadURL, fileContent); err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to upload configuration: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{
			"meta": map[string]any{
				"status": "uploaded",
				"cv_id":  c.ID,
				"bytes":  len(fileContent),
			},
		}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Configuration uploaded to %s (%d bytes).\n", c.ID, len(fileContent))
	}

	return nil
}

// defaultUploadClient uploads file content to a URL without Authorization header.
func defaultUploadClient(uploadURL string, fileContent []byte) error {
	req, err := http.NewRequest(http.MethodPut, uploadURL, nil)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(io.NewSectionReader(ioReaderAt(fileContent), 0, int64(len(fileContent))))
	req.ContentLength = int64(len(fileContent))
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("upload failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// ioReaderAt wraps a byte slice to implement io.ReaderAt.
type ioReaderAt []byte

func (r ioReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n = copy(p, r[off:])
	return n, nil
}

// CVDownloadCmd downloads configuration content from a configuration version.
type CVDownloadCmd struct {
	ID  string `arg:"" help:"ID of the configuration version."`
	Out string `help:"Write output to file instead of stdout."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
}

func (c *CVDownloadCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	content, err := client.Download(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to download configuration: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to download configuration: %w", err))
	}

	// Determine output format for meta output
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if c.Out != "" {
		// Write to file
		if err := os.WriteFile(c.Out, content, 0o644); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write file: %w", err))
		}

		// Emit meta JSON/table summary
		if format == output.FormatJSON {
			result := map[string]any{
				"meta": map[string]any{
					"written_to": c.Out,
					"bytes":      len(content),
				},
			}
			if err := output.WriteJSON(c.stdout, result); err != nil {
				return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
			}
		} else {
			fmt.Fprintf(c.stdout, "Configuration downloaded to %s (%d bytes).\n", c.Out, len(content))
		}
	} else {
		// Write to stdout
		if _, err := c.stdout.Write(content); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// CVArchiveCmd archives a configuration version.
type CVArchiveCmd struct {
	ID string `arg:"" help:"ID of the configuration version to archive."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory cvClientFactory
	prompter      ui.Prompter
}

func (c *CVArchiveCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultCVClientFactory
	}
	if c.prompter == nil {
		c.prompter = ui.NewStdPrompter(os.Stdin, os.Stdout)
	}

	// Confirm archive unless --force
	if !cli.Force {
		confirmed, err := c.prompter.Confirm(fmt.Sprintf("Archive configuration version %s?", c.ID), false)
		if err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for confirmation: %w", err))
		}
		if !confirmed {
			fmt.Fprintln(c.stdout, "Aborting archive.")
			return nil
		}
	}

	cfg, err := resolveCVClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	err = client.Archive(ctx, c.ID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to archive configuration version: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to archive configuration version: %w", err))
	}

	// Determine output format
	format, _ := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		result := map[string]any{
			"meta": map[string]any{
				"status": "archived",
				"cv_id":  c.ID,
			},
		}
		if err := output.WriteJSON(c.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		fmt.Fprintf(c.stdout, "Configuration version %s archived.\n", c.ID)
	}

	return nil
}
