package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// DoctorCmd validates settings, token discovery, and connectivity.
type DoctorCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory func(cfg tfcapi.ClientConfig) (doctorClient, error)
}

// doctorClient abstracts the TFC client for testing.
type doctorClient interface {
	Ping(ctx context.Context) error
}

// DoctorCheck represents a single doctor check result.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DoctorResult represents the full doctor output.
type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
}

func (d *DoctorCmd) Run(cli *CLI) error {
	// Set up defaults
	if d.tokenResolver == nil {
		d.tokenResolver = auth.NewTokenResolver()
	}
	if d.ttyDetector == nil {
		d.ttyDetector = &output.RealTTYDetector{}
	}
	if d.stdout == nil {
		d.stdout = os.Stdout
	}
	if d.clientFactory == nil {
		d.clientFactory = defaultDoctorClientFactory
	}

	// Determine output format
	format, isTTY := resolveFormat(d.stdout, d.ttyDetector, cli.OutputFormat)

	result := &DoctorResult{Checks: make([]DoctorCheck, 0)}
	hasFailure := false

	// Check 1: Settings file exists and is valid
	settings, err := config.Load(d.baseDir)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "settings",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("run 'tfc init': %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "settings",
		Status: string(output.StatusPass),
		Detail: "settings.json loaded",
	})

	// Resolve context (flag override or current)
	contextName := cli.Context
	if contextName == "" {
		contextName = settings.CurrentContext
	}
	ctx, exists := settings.Contexts[contextName]
	if !exists {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "context",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("context %q not found; run 'tfc contexts list' to see available contexts", contextName),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}

	// Apply defaults and overrides
	resolved := ctx.WithDefaults()
	if cli.Address != "" {
		resolved.Address = cli.Address
	}

	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "context",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("using context %q", contextName),
	})

	// Check 2: Address parsing
	hostname, err := auth.ExtractHostname(resolved.Address)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "address",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("invalid address %q: %v", resolved.Address, err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "address",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("hostname: %s", hostname),
	})

	// Check 3: Token resolution
	tokenResult, err := d.tokenResolver.ResolveToken(resolved.Address)
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "token",
			Status: string(output.StatusFail),
			Detail: err.Error(),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "token",
		Status: string(output.StatusPass),
		Detail: fmt.Sprintf("source: %s", tokenResult.Source),
	})

	// Check 4: Connectivity
	client, err := d.clientFactory(tfcapi.ClientConfig{
		Address: resolved.Address,
		Token:   tokenResult.Token,
	})
	if err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "connectivity",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("failed to create client: %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}

	pingCtx := context.Background()
	if err := client.Ping(pingCtx); err != nil {
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "connectivity",
			Status: string(output.StatusFail),
			Detail: fmt.Sprintf("API check failed: %v", err),
		})
		hasFailure = true
		return d.outputAndError(result, format, isTTY, hasFailure)
	}
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "connectivity",
		Status: string(output.StatusPass),
		Detail: "API reachable",
	})

	return d.outputAndError(result, format, isTTY, hasFailure)
}

func (d *DoctorCmd) outputAndError(result *DoctorResult, format output.Format, isTTY bool, hasFailure bool) error {
	if format == output.FormatJSON {
		if err := output.WriteJSON(d.stdout, result); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		tw := output.NewTableWriter(d.stdout, []string{"CHECK", "STATUS", "DETAIL"}, isTTY)
		for _, check := range result.Checks {
			tw.AddRow(check.Name, output.StatusStyle(output.Status(check.Status), isTTY), check.Detail)
		}
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	if hasFailure {
		return internalcmd.NewRuntimeError(errors.New("doctor checks failed"))
	}
	return nil
}

// defaultDoctorClientFactory creates a real TFC client that satisfies doctorClient.
func defaultDoctorClientFactory(cfg tfcapi.ClientConfig) (doctorClient, error) {
	return tfcapi.NewClientWithWrapper(cfg)
}
