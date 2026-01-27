# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is tfccli?

CLI for interacting with the Terraform Cloud APIs. Built with Go and Kong for command parsing.

## Commands

```bash
make              # Build binary to bin/tfccli
make test         # Run all tests
make lint         # Run golangci-lint
make fmt          # Format code (goimports + gofumpt)
make ci           # fmt-check + lint + test (CI pipeline)
make tools        # Install dev tools to .tools/
make tfccli ARGS="version"  # Build and run with args

go test ./internal/config/...  # Run tests for specific package
go test -run TestName ./...    # Run single test by name
```

## Architecture

```
cmd/tfc/main.go      # Entry point, Kong CLI struct, exit code handling
internal/
  config/            # Settings schema (~/.tfccli/settings.json), multi-context support
  cmd/               # Shared command utilities, RuntimeError type
  auth/              # Token discovery (placeholder)
  tfcapi/            # TFC API client (placeholder)
  output/            # Output formatting (placeholder)
  ui/                # User interaction (placeholder)
```

## Exit Codes

- 0: Success
- 1: Usage/parse error
- 2: Runtime error (wrap with `cmd.NewRuntimeError()`)
- 3: Unexpected/internal error

## Settings

Config stored at `~/.tfccli/settings.json`. Supports multiple named contexts with address, default_org, log_level. Default API address: `app.terraform.io`.
