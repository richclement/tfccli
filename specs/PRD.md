# PRD: `tfccli` — Terraform Cloud API CLI (Go)

## 1) Summary

`tfccli` is a Go-based CLI tool that exposes the **Terraform Cloud / HCP Terraform API** (and Terraform Enterprise via configurable address) through a consistent command-line interface that can be used by a human and by an AI coding agent. The CLI is designed to be **automation-friendly**, **testable**, and to follow **Terraform CLI credential discovery conventions**.

Repository: `richclement/tfccli` (private initially)

---

## 2) Goals

### Primary goals

* Provide a **first-class CLI wrapper** around core Terraform Cloud API resources:

  * `doctor`, `projects`, `workspaces`, `workspace-variables`, `workspace-resources`, `plans`, `runs`, `applies`, `configuration-versions`, `users`, `invoices`, `organizations`
* Support **CRUD operations** where the API supports them (create/update/delete in v1).
* Use **Terraform CLI conventions** for authentication token discovery (no custom token storage).
* Provide deterministic, agent-friendly outputs:

  * `--output-format table|json`
  * Default to **JSON** when stdout is not a TTY.
  * JSON output should be **raw JSON:API documents** (stable contract).
* Be safe for destructive operations:

  * interactive confirmation by default
  * `--force` to bypass prompts

### Secondary goals

* Support **multiple contexts** (profiles) with a “current context” selection.
* Provide a robust API client:

  * retry/backoff on transient failures and 429
  * auto-pagination to retrieve full lists

---

## 3) Non-goals (v1)

* Homebrew packaging wiring (explicitly out of scope; will be handled separately).
* Credential helpers (Terraform CLI `credentials_helper` support) — skip in v1.
* “Search” features (fuzzy search / query filtering) — skip for now.
* Deep `doctor` diagnostics beyond “settings + token + basic connectivity.”

---

## 4) Users / Personas

### Primary persona: Developer/operator (Rich)

* Wants a fast, scriptable CLI to manage TFC/TFE resources and drive workflows.

### Secondary persona: Coding agent

* Needs stable output contracts, consistent exit codes, and non-interactive modes for config and mutations.

---

## 5) Key User Stories

1. **Initialize CLI configuration**

   * As a user/agent, I can run `tfccli init` to create `~/.tfccli/settings.json` with a default address and log level.

2. **Verify environment**

   * As a user/agent, I can run `tfccli doctor` to validate that settings exist and a Terraform API token can be discovered using Terraform conventions.

3. **Operate resources by ID**

   * As a user/agent, I can `get/update/delete` resources by their API IDs.

4. **Automate**

   * As an agent, I can run commands non-interactively and parse JSON output consistently.

---

## 6) CLI UX Requirements

### Command name

* Root command: `tfccli`

### Global flags

* `--context <name>`: select a context from settings
* `--address <addr>`: override context address for this invocation
* `--org <org>`: override context `default_org` for this invocation
* `--output-format table|json`: override output format
* `--debug`: enable debug logging for this invocation
* `--force`: bypass confirmation prompts for destructive operations

### Exit codes (consistent)

* `0`: success
* `1`: CLI usage / validation errors (missing required flags, invalid enums)
* `2`: runtime errors (config missing/invalid, auth missing, API errors)
* `3`: unexpected errors/panics (should be rare)

### Output formatting

* `--output-format=json`:

  * Emit **raw JSON:API** documents when API returns JSON:API.
  * For 204/empty success responses: emit a small JSON object like `{"meta":{"status":204}}` to remain machine-readable.
* `--output-format=table`:

  * Human-readable columns for common resources (id/name/status where applicable).
  * Use `termenv` for styling only when stdout is a TTY.

### Defaults

* If stdout is a TTY: default output format is `table`
* If stdout is not a TTY: default output format is `json`

---

## 7) Configuration Requirements

### Location

* Directory: `~/.tfccli/`
* File: `~/.tfccli/settings.json`

### Multi-context model

Settings must support multiple named contexts and track which one is current.

**Required fields**

* `current_context`: string
* `contexts`: object keyed by context name

**Context fields**

* `address` (string): **full base address**; may include host + path (e.g. `app.terraform.io/eu`, `https://tfe.example.com`)

  * If scheme is omitted, default to `https://`
* `default_org` (string, optional): used when commands require org scope and `--org` is not provided
* `log_level` (`debug|info|warn|error`): default `info`

### `tfccli init`

* Creates the folder and settings.json.
* Interactive:

  * prompts for `address` (default `app.terraform.io`)
  * prompts for `default_org` (optional)
  * prompts for `log_level` (default `info`)
* Non-interactive mode supported (for agents/CI), e.g.:

  * `tfccli init --non-interactive --address ... --default-org ... --log-level ... --yes`
* If settings already exist:

  * do not overwrite unless explicitly confirmed or `--yes` is set

### Context management (required due to “multiple contexts” decision)

Provide commands to manage contexts:

* `tfccli contexts list`
* `tfccli contexts add <name> --address ... [--default-org ...] [--log-level ...]`
* `tfccli contexts use <name>`
* `tfccli contexts remove <name> [--force]`
* `tfccli contexts show [<name>]`

---

## 8) Authentication Requirements (Terraform CLI conventions)

`tfccli` **must not** store tokens in `settings.json`.

Token discovery must follow Terraform CLI conventions, with this precedence:

1. **Environment variable**: `TF_TOKEN_<sanitized_host>`

   * Example: `TF_TOKEN_app_terraform_io` for `app.terraform.io`.
2. **Terraform CLI config** credentials blocks (honoring `TF_CLI_CONFIG_FILE` when present).
3. **Terraform login credentials file** `credentials.tfrc.json` (commonly under `~/.terraform.d/`).

Explicitly out of scope for v1:

* `credentials_helper` integration

Behavior:

* If no token can be discovered, return a clear actionable error (e.g., suggest `terraform login`).

---

## 9) Address / Hostname Semantics

* `address` is user-configured and may include:

  * host only: `app.terraform.io`
  * host + path: `app.terraform.io/eu`
  * full URL: `https://tfe.example.com`

* `tfccli` must derive:

  * **API base URL**: `<address>/api/v2` (respecting any path component)
  * **hostname for token lookup**: the host portion only (ignore path)

---

## 10) API Client Requirements

### Library usage

* Prefer `github.com/hashicorp/go-tfe` when possible.

### Reliability

* Auto-pagination for list endpoints (“fetch all”) by default.
* Retry/backoff:

  * Retry transient 5xx and 429 responses
  * Honor `Retry-After` header when present
  * Bounded attempts (avoid infinite retries)

### Redirect handling (security)

Some download-style endpoints return redirects (e.g., plan JSON output, sanitized plan, errored state, configuration version downloads/uploads):

* Follow redirects automatically.
* **Do not forward the Terraform API token** (Authorization header) to the redirected host.

---

## 11) Logging Requirements

* Use `logr` interface with `stdr` implementation.
* Default log level: `info` (from settings).
* `--debug` overrides to debug for the invocation.
* Logs must never include secrets:

  * API tokens
  * sensitive variable values
* Debug logs may include:

  * request method/path
  * status code
  * retry attempt counts
  * request IDs (if available)

---

## 12) Command Scope and Requirements (v1)

Each command group must support JSON and table outputs and consistent error behavior.

### `tfccli doctor`

* Validate settings existence and parseability
* Validate context selection
* Validate token discovery (and report source, without printing the token)
* Basic connectivity check (lightweight API call)

### `tfccli organizations`

* list/get/create/update/delete (with confirmation + `--force`)

### `tfccli projects`

* Org-scoped list/create (uses `--org` or `default_org`)
* get/update/delete by ID

### `tfccli workspaces`

* Org-scoped list/create (uses `--org` or `default_org`)
* get/update/delete by ID
* create supports optional `--project-id` flag

### `tfccli workspace-variables`

* list/create/update/delete
* create supports categories `env|terraform`, plus `--sensitive` and `--hcl` flags

### `tfccli workspace-resources`

* list (read-only)

### `tfccli configuration-versions`

* list/get/create/upload/download/archive
* upload uses the returned upload URL and should not attach Authorization

### `tfccli runs`

* list (at minimum workspace-scoped), get, create
* actions: apply/discard/cancel/force-cancel (destructive confirmation + `--force`)

### `tfccli plans`

* get
* json-output download
* sanitized-plan download

### `tfccli applies`

* get
* errored-state download

### `tfccli users`

* get by ID
* me (get current authenticated user via `/account/details`)

### `tfccli invoices`

* list/get/next as supported by the API
* org-scoped where required (uses `--org` or `default_org`)

---

## 13) Security and Safety Requirements

* Never store tokens in `~/.tfccli/settings.json`.
* Never print tokens to stdout/stderr/logs.
* Never forward tokens to redirect hosts.
* Destructive actions require confirmation unless `--force` is set.
* Sensitive variable values must not appear in debug logs.

---

## 14) Testability Requirements

Design must enable unit tests with minimal friction:

* Injectable abstractions for:

  * filesystem paths / HOME resolution (use temp homes in tests)
  * TTY detection (to test output defaults)
  * prompter/confirmation IO (scripted answers)
  * HTTP client/transport (use `httptest` to record requests)
* Commands should be testable at two levels:

  1. unit tests for helpers (token discovery, config parsing, address normalization)
  2. command tests using a fake API server (verify method/path/query/headers/body + outputs)

All tasks should include Gherkin scenarios convertible to Go unit tests.

---

## 15) Dependencies (explicit)

* CLI parsing: `github.com/alecthomas/kong`
* Terminal styling: `github.com/muesli/termenv`
* Logging: `github.com/go-logr/logr`, `github.com/go-logr/stdr`
* API client: `github.com/hashicorp/go-tfe`

---

## 16) Release Requirements

* Use GoReleaser to build cross-platform binaries and attach build metadata (`tfccli version`).
* Homebrew tap wiring is out of scope (handled separately).

---

## 17) Implementation Guidance (for humans/agents picking up a single task)

When executing any task, ensure:

* You adhere to global UX rules (flags, output, exit codes).
* JSON output uses raw JSON:API where applicable.
* Destructive commands enforce confirmation/`--force`.
* You do not introduce token storage/logging regressions.
* You keep code structured so subsequent command tasks can reuse:

  * config resolution
  * token discovery
  * API client setup
  * pagination
  * output logic

Suggested internal package boundaries:

* `internal/config` (settings + contexts)
* `internal/auth` (token discovery)
* `internal/tfcapi` (client wiring, retry, pagination, redirect-safe download)
* `internal/output` (json/table selection + rendering)
* `internal/ui` (prompting, confirmation, tty checks)
* `internal/cmd/*` (per-command implementations)

---

## 18) Future Considerations (post-v1)

* Add “search” behaviors (server-side filters + local fuzzy search).
* Add credentials_helper support if needed.
* Expand doctor diagnostics (proxy/TLS, org membership checks, permissions introspection).
* Expand users command set (token management endpoints if required).

---

If you want, I can also generate a **one-page “Task Template”** (Definition of Done + checklist + example unit test structure) that every agent task PR should follow for consistency.
