package main

import (
	"fmt"
	"io"
	"os"

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// resolveClientConfig resolves settings and token for API calls, including org resolution.
// Returns the client config, resolved org name, and any error.
func resolveClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, string, error) {
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

// resolveFormat determines the output format based on CLI flags and TTY detection.
// Returns both the format and the isTTY bool (needed by TableWriter).
func resolveFormat(stdout io.Writer, ttyDetector output.TTYDetector, cliFormat string) (output.Format, bool) {
	isTTY := false
	if f, ok := stdout.(*os.File); ok {
		isTTY = ttyDetector.IsTTY(f)
	}
	return output.ResolveOutputFormat(cliFormat, isTTY), isTTY
}
