# Architecture: tfc CLI

## Purpose and scope

`tfccli` is a Go CLI for Terraform Cloud / HCP Terraform (and TFE via custom address). It is built for:

- automation-friendly, deterministic output (JSON default for non-TTY)
- Terraform CLI token discovery conventions
- safe destructive operations (`--force` to bypass prompts)

This document maps how the codebase is organized and how requests flow end-to-end.

## High-level flow

```
main() -> kong parse -> build logger + signal-aware context
  -> command.Run(cli)
     -> resolve settings + context overrides
     -> resolve token (env / TF config / credentials file)
     -> build API client(s)
     -> perform API call(s)
     -> format output (json/table) + exit code
```

## Repository map

- `cmd/tfc/`: CLI entrypoint + all command implementations
- `internal/config/`: settings file model, load/save/validate, defaults
- `internal/auth/`: token discovery + address/hostname parsing
- `internal/tfcapi/`: go-tfe client wiring, pagination, error parsing, HTTP helpers
- `internal/output/`: JSON/table rendering, output format selection, TTY detection
- `internal/ui/`: prompting/confirmation abstractions
- `internal/logging/`: logr/stdr logger factory
- `internal/testutil/`: test helpers (fake env/fs, prompters, request recorder, temp home)
- `specs/`: product requirements and task lists
- `tests/`: e2e shell tests
- `docs/`: release and architecture docs

## Entry point and CLI wiring

- `cmd/tfc/main.go` wires Kong, builds a logger, and installs signal handling.
- Global flags live on `CLI` (context/address/org/output-format/debug/force).
- Each subcommand is a struct with `Run(*CLI) error`.
- Exit code mapping:
  - 0 success
  - 1 parse/usage errors (Kong)
  - 2 runtime errors (`internal/cmd.RuntimeError`)
  - 3 unexpected errors/panics

## Command pattern (shared behavior)

Most commands follow the same structure:

1. Set defaults for injected dependencies (stdout, ttyDetector, prompter, clientFactory).
2. Resolve settings + token via `resolveClientConfig` (or `resolveClientConfigWithRequiredOrg`).
3. Call a client method (go-tfe or raw HTTP).
4. Convert to JSON-safe structs (avoid go-tfe NullableAttr).
5. Render output based on resolved format.
6. Wrap errors with `internal/cmd.NewRuntimeError`.

Key helpers live in `cmd/tfc/common.go`:

- `resolveClientConfig` loads settings, applies overrides, resolves token.
- `resolveFormat` selects json/table based on `--output-format` + TTY.
- `cmdContext` uses the signal-aware context from `CLI`.

## Configuration model

Implemented in `internal/config`:

- Settings path: `~/.tfccli/settings.json`
- Schema:
  - `current_context` (string)
  - `contexts` map of `Context`
    - `address`, `default_org`, `log_level`
- Defaults: address `app.terraform.io`, log level `info`.
- `init` and `contexts` commands read/write settings via `config.Load/Save`.

## Authentication/token discovery

Implemented in `internal/auth`:

- Token discovery order:
  1. Env var `TF_TOKEN_<sanitized_host>`
  2. Terraform CLI config (`TF_CLI_CONFIG_FILE` or `~/.terraformrc`)
  3. `~/.terraform.d/credentials.tfrc.json`
- `ExtractHostname` accepts host, host+path, or URL.
- Tokens are never stored in settings and are never logged.

## API clients and HTTP behavior

Implemented in `internal/tfcapi` and per-command clients:

- Primary client: `github.com/hashicorp/go-tfe` (`tfcapi.NewClient`).
- Address normalization: `NormalizeAddress` adds `https://` if missing.
- API base URL: `APIBaseURL(address) => <addr>/api/v2`.
- Pagination helpers: `CollectAll*` functions return full lists.
- Error parsing: `ParseAPIError` wraps JSON:API errors and go-tfe errors.
- Raw HTTP clients:
  - `users` uses `internal/tfcapi.HTTPClient` (go-tfe lacks endpoints).
  - `invoices` uses a custom HTTP client with cursor pagination and JSON:API error parsing.
- Download/upload flows avoid forwarding Authorization on signed URLs:
  - Plans sanitized plan download
  - Applies errored state download (captures redirect URL first)
  - Configuration version uploads (PUT to upload URL)

## Output formats

Implemented in `internal/output`:

- `ResolveOutputFormat`:
  - explicit `--output-format` wins
  - default `table` on TTY, `json` otherwise
- JSON output:
  - `WriteJSON` emits indented JSON
  - `WriteEmptySuccess` for 204/202 responses
  - Most resources wrap data as `{"data": ...}`
  - Some commands pass through raw JSON:API (users, invoices)
- Table output:
  - `TableWriter` aligns columns and styles headers when TTY
  - `StatusStyle` for doctor checks

## Logging

Implemented in `internal/logging`:

- Uses logr + stdr
- Verbosity derived from settings `log_level`, overridden by `--debug`
- Logs go to stderr

## Testing strategy

- Unit tests live alongside code (`*_test.go`).
- Command structs support dependency injection:
  - `baseDir` (fake HOME)
  - `tokenResolver` (fake env/fs)
  - `ttyDetector`, `stdout`
  - `clientFactory` (mock clients)
  - `prompter` (accept/reject/fail)
- `internal/testutil` provides:
  - `TempHome`, `DefaultTestSettings`, `MultiContextSettings`
  - Fake env/fs + token resolver
  - Request recorder for HTTP tests
  - Accepting/Rejecting/Failing prompters
- E2E read-only script: `tests/e2e/read_commands_test.sh`

## Adding a new command (recommended pattern)

1. Add command struct to `cmd/tfc/main.go` CLI.
2. Implement `Run(*CLI) error` in `cmd/tfc/<resource>.go`.
3. Use:
   - `resolveClientConfig` / `resolveClientConfigWithRequiredOrg`
   - `resolveFormat` and `output.WriteJSON` / `TableWriter`
   - `internal/cmd.NewRuntimeError` for runtime failures
4. For destructive actions, prompt unless `--force`.
5. Prefer go-tfe client; use raw HTTP only when go-tfe lacks the endpoint.

## Guardrails and invariants

- Never store or print tokens.
- Do not forward Authorization to signed download URLs.
- Destructive operations require confirmation unless `--force`.
- Default output must remain deterministic for agents (JSON on non-TTY).
