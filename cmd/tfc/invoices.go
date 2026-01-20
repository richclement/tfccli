package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// InvoicesCmd groups all invoices subcommands.
type InvoicesCmd struct {
	List InvoicesListCmd `cmd:"" help:"List invoices for an organization."`
	Next InvoicesNextCmd `cmd:"" help:"Get the next (upcoming) invoice for an organization."`
}

// invoicesClient abstracts the TFC invoices API for testing.
type invoicesClient interface {
	List(ctx context.Context, org string) (*InvoicesListResponse, error)
	GetNext(ctx context.Context, org string) (*InvoiceResponse, error)
}

// InvoicesListResponse represents the JSON:API response for listing invoices.
type InvoicesListResponse struct {
	Data []InvoiceData    `json:"data"`
	Meta *InvoiceListMeta `json:"meta,omitempty"`
}

// InvoiceListMeta contains pagination metadata for invoice list.
type InvoiceListMeta struct {
	Continuation string `json:"continuation,omitempty"`
}

// InvoiceResponse represents the JSON:API response for a single invoice.
type InvoiceResponse struct {
	Data InvoiceData `json:"data"`
}

// InvoiceData represents an invoice in JSON:API format.
type InvoiceData struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Attributes InvoiceAttributes `json:"attributes"`
}

// InvoiceAttributes represents invoice attributes.
type InvoiceAttributes struct {
	CreatedAt    time.Time `json:"created-at"`
	ExternalLink string    `json:"external-link,omitempty"`
	Number       string    `json:"number"`
	Paid         bool      `json:"paid"`
	Status       string    `json:"status"`
	Total        int       `json:"total"` // amount in cents
}

// invoicesClientFactory creates an invoicesClient from config.
type invoicesClientFactory func(cfg tfcapi.ClientConfig) (invoicesClient, error)

// realInvoicesClient implements invoicesClient using raw HTTP calls.
// go-tfe doesn't expose invoices API, so we make the calls directly.
type realInvoicesClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func (c *realInvoicesClient) List(ctx context.Context, org string) (*InvoicesListResponse, error) {
	// Collect all pages (cursor-based pagination)
	var allInvoices []InvoiceData
	var continuation string

	for {
		apiURL := fmt.Sprintf("%s/api/v2/organizations/%s/invoices", c.baseURL, url.PathEscape(org))
		if continuation != "" {
			apiURL = fmt.Sprintf("%s?page[cursor]=%s", apiURL, url.QueryEscape(continuation))
		}

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/vnd.api+json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if err := c.handleErrorResponse(resp.StatusCode, body); err != nil {
			return nil, err
		}

		var listResp InvoicesListResponse
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		allInvoices = append(allInvoices, listResp.Data...)

		// Check for more pages
		if listResp.Meta == nil || listResp.Meta.Continuation == "" {
			break
		}
		continuation = listResp.Meta.Continuation
	}

	return &InvoicesListResponse{Data: allInvoices}, nil
}

func (c *realInvoicesClient) GetNext(ctx context.Context, org string) (*InvoiceResponse, error) {
	apiURL := fmt.Sprintf("%s/api/v2/organizations/%s/invoices/next", c.baseURL, url.PathEscape(org))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if err := c.handleErrorResponse(resp.StatusCode, body); err != nil {
		return nil, err
	}

	var invoiceResp InvoiceResponse
	if err := json.Unmarshal(body, &invoiceResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &invoiceResp, nil
}

// handleErrorResponse handles HTTP error responses.
func (c *realInvoicesClient) handleErrorResponse(statusCode int, body []byte) error {
	if statusCode == http.StatusOK {
		return nil
	}

	if statusCode == http.StatusNotFound {
		// Check if this is an "invoices not available" error
		if strings.Contains(string(body), "invoices") ||
			strings.Contains(string(body), "not found") ||
			strings.Contains(string(body), "Not Found") {
			return &invoicesNotAvailableError{}
		}
		return fmt.Errorf("resource not found")
	}

	if statusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: invalid or missing API token")
	}

	if statusCode == http.StatusForbidden {
		// Invoices API may return 403 for orgs without billing access
		return &invoicesNotAvailableError{}
	}

	// Try to parse JSON:API error
	var errResp struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
		return fmt.Errorf("%s: %s", errResp.Errors[0].Title, errResp.Errors[0].Detail)
	}

	return fmt.Errorf("API request failed with status %d", statusCode)
}

// invoicesNotAvailableError indicates that the invoices API is not available.
type invoicesNotAvailableError struct{}

func (e *invoicesNotAvailableError) Error() string {
	return "invoices API is only available in HCP Terraform (Cloud) for credit-card-billed plans"
}

// defaultInvoicesClientFactory creates a real TFC client that satisfies invoicesClient.
func defaultInvoicesClientFactory(cfg tfcapi.ClientConfig) (invoicesClient, error) {
	baseURL := tfcapi.NormalizeAddress(cfg.Address)
	if baseURL == "" {
		baseURL = "https://app.terraform.io"
	}

	return &realInvoicesClient{
		baseURL:    baseURL,
		token:      cfg.Token,
		httpClient: &http.Client{},
	}, nil
}

// resolveInvoicesClientConfig resolves settings, token, and org for API calls.
func resolveInvoicesClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, string, error) {
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

	// Resolve organization
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

// InvoicesListCmd lists invoices for an organization.
type InvoicesListCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory invoicesClientFactory
}

func (c *InvoicesListCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultInvoicesClientFactory
	}

	cfg, org, err := resolveInvoicesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if org == "" {
		// Exit code 1 for usage error (missing required parameter)
		return fmt.Errorf("organization is required: use --org flag or set default_org in context")
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	invoices, err := client.List(ctx, org)
	if err != nil {
		// Check for invoices not available error
		if _, ok := err.(*invoicesNotAvailableError); ok {
			return internalcmd.NewRuntimeError(err)
		}
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to list invoices: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to list invoices: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		// Emit JSON:API-like response
		if err := output.WriteJSON(c.stdout, invoices); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		// Table output
		tw := output.NewTableWriter(c.stdout, []string{"ID", "STATUS", "NUMBER", "TOTAL", "PAID", "CREATED"}, isTTY)
		for _, inv := range invoices.Data {
			totalDollars := fmt.Sprintf("$%.2f", float64(inv.Attributes.Total)/100)
			paid := "no"
			if inv.Attributes.Paid {
				paid = "yes"
			}
			tw.AddRow(
				inv.ID,
				inv.Attributes.Status,
				inv.Attributes.Number,
				totalDollars,
				paid,
				inv.Attributes.CreatedAt.Format("2006-01-02"),
			)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}

// InvoicesNextCmd gets the next (upcoming) invoice for an organization.
type InvoicesNextCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory invoicesClientFactory
}

func (c *InvoicesNextCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultInvoicesClientFactory
	}

	cfg, org, err := resolveInvoicesClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	if org == "" {
		// Exit code 1 for usage error (missing required parameter)
		return fmt.Errorf("organization is required: use --org flag or set default_org in context")
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	invoice, err := client.GetNext(ctx, org)
	if err != nil {
		// Check for invoices not available error
		if _, ok := err.(*invoicesNotAvailableError); ok {
			return internalcmd.NewRuntimeError(err)
		}
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get next invoice: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get next invoice: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		// Emit JSON:API-like response
		if err := output.WriteJSON(c.stdout, invoice); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		// Table output: show key fields in FIELD/VALUE format
		inv := invoice.Data
		totalDollars := fmt.Sprintf("$%.2f", float64(inv.Attributes.Total)/100)
		paid := "no"
		if inv.Attributes.Paid {
			paid = "yes"
		}

		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", inv.ID)
		tw.AddRow("Status", inv.Attributes.Status)
		tw.AddRow("Number", inv.Attributes.Number)
		tw.AddRow("Total", totalDollars)
		tw.AddRow("Paid", paid)
		tw.AddRow("Created", inv.Attributes.CreatedAt.Format("2006-01-02"))
		if inv.Attributes.ExternalLink != "" {
			tw.AddRow("External Link", inv.Attributes.ExternalLink)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}
