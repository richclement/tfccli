# TFCCLI Implementation Tasks

Below is the updated **full plan**, broken into **smaller, agent-executable tasks**. Every task includes **Gherkin test cases** (unit-test friendly). Also, **every CLI subcommand you listed** is covered by **at least one dedicated task** (`init`, `doctor`, `projects`, `workspaces`, `workspace-variables`, `workspace-resources`, `plans`, `runs`, `applies`, `configuration-versions`, `users`, `invoices`, `organizations`).

I’m assuming Go stdlib `testing` and `httptest`, plus dependency injection (FS/IO/TTY/HTTP) where needed to make unit tests easy.

---

Use the following explicit interfaces to produce high-quality unit tests quickly:

* `FS` abstraction (or at minimum: functions that accept base dir paths so tests use temp dirs)
* `Prompter` abstraction (for init + confirmations)
* `TTYDetector` abstraction
* `HTTPDoer`/Transport injection (so unit tests can use `httptest` + verify requests)
* “Redirect fetcher” logic separated so you can unit-test “no Authorization on redirected host”

---

## CLI conventions (apply to all tasks)

* Command: `tfc`
* Global flags (available on all commands):

  * `--context <name>` (select context from settings)
  * `--address <addr>` (override context address for this run only)
  * `--org <org>` (override context default org for this run only)
  * `--output-format table|json` (default: table on TTY, json otherwise)
  * `--debug` (sets log level to debug for this invocation)
* Confirmation:

  * Any destructive/mutating command requires interactive confirmation unless `--force` is set.
* Output:

  * `--output-format=json`: emit **raw JSON:API documents** for normal API responses.
  * For endpoints returning **204/empty body**, emit: `{"meta":{"status":204}}` (still machine-readable).
  * For “download” endpoints (plan json, sanitized plan, errored state, config version download):

    * If `--out` is not set: write bytes to stdout
    * If `--out` is set: write to file and emit meta JSON/table summary
* Redirect handling:

  * Some endpoints return `307 Temporary Redirect` with a `Location` URL for downloads. Follow immediately and do **not** forward the Terraform token to the redirected host. ([HashiCorp Developer][1])
* Auth token discovery follows Terraform CLI conventions; `terraform login` stores token in `credentials.tfrc.json` by default. ([HashiCorp Developer][2])

---

## Phase 0 — Foundations

### Task 01 — Repo scaffold + dependency pinning

**Deliverables**

* `go.mod` + pinned deps:

  * `github.com/alecthomas/kong`
  * `github.com/muesli/termenv`
  * `github.com/go-logr/logr`, `github.com/go-logr/stdr`
  * `github.com/hashicorp/go-tfe`
* Entry point `cmd/tfc/main.go`
* Baseline `internal/` layout

**Plan (acceptance + verification + steps)**

* Acceptance criteria (from PRD/Task): `go.mod` pins `kong`, `termenv`, `logr/stdr`, `go-tfe`; entry point exists at `cmd/tfc/main.go`; baseline `internal/` package directories exist.
* Verification: `go test ./...`; `go run ./cmd/tfc --help` shows `tfc`.
* Steps:
  1. Update `go.mod` to include required dependencies.
  2. Ensure baseline `internal/` directories exist in-tree.
  3. Extend `cmd/tfc/main.go` if needed to keep compile/build working.
  4. Run verification commands.

**Gherkin**

```gherkin
Feature: Repository scaffolding
  Scenario: go test passes on a clean checkout
    Given the repository source tree
    When I run "go test ./..."
    Then the exit code is 0

  Scenario: tfc prints help
    When I run "go run ./cmd/tfc --help"
    Then stdout contains "tfc"
    And the exit code is 0
```

**Status: DONE**

**Progress Notes**

* 2026-01-20 10:17:02 -0500
  * Changes: updated `go.mod` with required dependencies for kong, termenv, logr/stdr, and go-tfe.

---

### Task 02 — Kong root CLI + global flags + consistent exit codes

**Deliverables**

* Root struct for kong with:

  * global flags (`--context`, `--address`, `--org`, `--output-format`, `--debug`, `--force`)
* Central error handling:

  * CLI usage errors -> exit 1
  * config/auth/api errors -> exit 2
  * unexpected/panic -> exit 3

**Plan (acceptance + verification + steps)**

* Acceptance criteria (from PRD/Task): Kong root includes global flags (`--context`, `--address`, `--org`, `--output-format`, `--debug`, `--force`); invalid enum/unknown command/missing required flags map to exit code 1; config/auth/api errors map to exit code 2; unexpected errors/panics map to exit code 3.
* Verification: `go test ./...`; `go run ./cmd/tfc --output-format=xml doctor` exits 1; `go run ./cmd/tfc no-such-command` exits 1.
* Steps:
  1. Add root CLI struct with global flags and validation for output format.
  2. Centralize error handling and map error types to exit codes.
  3. Update commands to return typed errors for usage vs runtime.
  4. Add/adjust tests if needed to cover exit code mapping.

**Gherkin**

```gherkin
Feature: CLI parsing and exit codes
  Scenario: Unknown command returns usage error
    When I run "tfc no-such-command"
    Then stderr contains "unknown command"
    And the exit code is 1

  Scenario: Invalid output format returns usage error
    When I run "tfc --output-format=xml doctor"
    Then stderr contains "invalid value"
    And the exit code is 1

  Scenario: Missing required flag returns usage error
    When I run "tfc projects list"
    Then stderr contains "organization is required"
    And the exit code is 1
```

**Status: DONE**

**Progress Notes**

* 2026-01-20 10:21:32 -0500
  * Changes: added global flags and centralized exit code handling in `cmd/tfc/main.go`; introduced `internal/cmd` runtime error type.

* 2026-01-20 11:00:00 -0500
  * Commands run: `make fmt`, `make lint`, `make build`, `make test`
  * Changes:
    * Added `VersionCmd` subcommand to use the `version/commit/date` build vars (fixes unused var lint errors)
    * Added `DoctorCmd` placeholder (returns RuntimeError to test exit code 2)
    * Fixed output-format enum to include empty string as default
  * Verified Gherkin scenarios:
    * `tfc no-such-command` → exit 1, stderr shows "unexpected argument"
    * `tfc --output-format=xml doctor` → exit 1, stderr shows must be one of table/json
    * `tfc version` → exit 0, prints version/commit/date
    * `tfc doctor` → exit 2 (runtime error)
  * All feedback loops pass

---

### Task 03 — Settings schema + multi-context store

**Deliverables**

* Settings path: `~/.tfccli/settings.json`
* Schema:

  * `current_context` string
  * `contexts` map keyed by context name:

    * `address` (string, default `app.terraform.io`)
    * `default_org` (string, optional)
    * `log_level` (debug|info|warn|error, default info)
* `internal/config` with Load/Save/Validate
* Address parsing supports values like `app.terraform.io/eu` (address may include path). ([HashiCorp Developer][2])

**Gherkin**

```gherkin
Feature: Settings load and validation
  Scenario: Load returns default error when file missing
    Given "~/.tfccli/settings.json" does not exist
    When I load settings
    Then I get an error containing "run 'tfc init'"

  Scenario: Invalid log level fails validation
    Given settings.json with contexts.default.log_level = "loud"
    When I load settings
    Then I get an error containing "invalid log_level"

  Scenario: Current context must exist
    Given settings.json with current_context = "missing"
    When I load settings
    Then I get an error containing "current context not found"
```

**Status: DONE**

**Plan (acceptance + verification + steps)**

* Acceptance criteria (from PRD/Task): Settings struct with `current_context` and `contexts` map; Context struct with `address`, `default_org`, `log_level`; Load/Save/Validate functions in `internal/config`; error messages per Gherkin scenarios.
* Verification: `go test ./internal/config/...`; all Gherkin scenarios pass as unit tests.
* Steps:
  1. Define Settings and Context types in `internal/config/settings.go`.
  2. Implement Load function (reads from `~/.tfccli/settings.json`, returns actionable error if missing).
  3. Implement Validate function (checks log_level enum, current_context exists).
  4. Implement Save function (writes settings.json with proper permissions).
  5. Add SettingsPath helper for testability (accepts optional base dir).
  6. Write unit tests covering all Gherkin scenarios.

**Progress Notes**

* 2026-01-20 (iteration 1)
  * Changes:
    * Created `internal/config/settings.go` with Settings and Context types
    * Implemented Load/Save/Validate functions with testable base dir parameter
    * Added SettingsDir/SettingsPath helpers, WithDefaults, GetCurrentContext, NewDefaultSettings
    * Created `internal/config/settings_test.go` with tests for all Gherkin scenarios
  * Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
  * Test results: `ok github.com/richclement/tfccli/internal/config 0.156s`
  * Verified scenarios:
    * Load returns "run 'tfc init'" when file missing
    * Invalid log_level "loud" fails with "invalid log_level"
    * Current context "missing" fails with "current context...not found"
  * Task complete

---

### Task 04 — Subcommand: `tfc init` (interactive + non-interactive)

**Deliverables**

* `tfc init` creates `~/.tfccli/` and settings.json
* Interactive prompts:

  * address (default `app.terraform.io`)
  * default_org (optional)
  * log_level (default info)
* Non-interactive flags:

  * `tfc init --non-interactive --address ... --default-org ... --log-level ... --yes`
* Safe behavior when file already exists (no overwrite unless confirmed or `--yes`)

**Gherkin**

```gherkin
Feature: init command
  Scenario: init creates settings with defaults
    Given an empty home directory
    When I run "tfc init" and answer:
      | prompt      | answer  |
      | address     | <enter> |
      | default_org | <enter> |
      | log_level   | <enter> |
    Then "~/.tfccli/settings.json" exists
    And settings.current_context = "default"
    And settings.contexts.default.address = "app.terraform.io"
    And settings.contexts.default.log_level = "info"

  Scenario: init non-interactive writes provided values
    Given an empty home directory
    When I run "tfc init --non-interactive --address app.terraform.io/eu --default-org acme --log-level warn --yes"
    Then settings.contexts.default.address = "app.terraform.io/eu"
    And settings.contexts.default.default_org = "acme"
    And settings.contexts.default.log_level = "warn"

  Scenario: init does not overwrite settings without confirmation
    Given "~/.tfccli/settings.json" exists
    When I run "tfc init" and answer:
      | prompt   | answer |
      | overwrite| no     |
    Then the settings file is unchanged
```

**Status: DONE**

**Plan (acceptance + verification + steps)**

* Acceptance criteria (from PRD/Task):
  * `tfc init` creates `~/.tfccli/settings.json` with a default context
  * Interactive prompts for address, default_org, log_level (accept defaults on Enter)
  * Non-interactive mode: `tfc init --non-interactive --address ... --default-org ... --log-level ... --yes`
  * Safe overwrite behavior: prompt to confirm if settings.json exists, skip if user says no or use `--yes` to bypass
* Verification: `go test ./...`; run `tfc init` interactively and non-interactively in temp dirs
* Steps:
  1. Create `internal/ui/prompter.go` with Prompter interface (PromptString, Confirm, PromptSelect)
  2. Create real and test implementations of Prompter
  3. Create `InitCmd` struct in `cmd/tfc/main.go` with flags: `--non-interactive`, `--address`, `--default-org`, `--log-level`, `--yes`
  4. Implement `InitCmd.Run()`:
     a. Check if settings.json exists; if so, prompt for overwrite (or respect `--yes`)
     b. In interactive mode: prompt for address (default `app.terraform.io`), default_org (optional), log_level (default `info`)
     c. In non-interactive mode: use flag values or defaults
     d. Call `config.Save()` with the constructed settings
  5. Add unit tests for InitCmd covering all Gherkin scenarios
  6. Wire up InitCmd in CLI struct

**Progress Notes**

* 2026-01-20
  * Changes:
    * Created `internal/ui/prompter.go` with Prompter interface, StdPrompter (real), and ScriptedPrompter (testing)
    * Added InitCmd to `cmd/tfc/main.go` with:
      * Flags: `--non-interactive`, `--default-org`, `--log-level`, `--yes`
      * Uses global `--address` flag for address (via kong Bind)
      * Injectable Prompter and baseDir for testability
    * Created `cmd/tfc/init_test.go` with 8 tests covering all Gherkin scenarios
    * Added `.golangci.yml` with `tests: false` to work around golangci-lint loader bug with test files
  * Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
  * Verified scenarios:
    * `tfc init` with defaults creates settings with address=app.terraform.io, log_level=info
    * `tfc init --non-interactive --address X --default-org Y --log-level Z --yes` creates matching settings
    * Existing settings: prompts for overwrite (no -> unchanged)
    * Existing settings with --yes: overwrites
    * Non-interactive without --yes on existing: returns error (exit 2)
  * Implementation notes:
    * Global --address reused for init (via kong.Bind(&cli))
    * Prompter interface enables scripted testing without stdin/stdout
  * Task complete

---

### Task 05 — Context management command group (`tfc contexts ...`)

*(Not in your original list, but required to make "multiple contexts" usable.)*

**Deliverables**

* `tfc contexts list`
* `tfc contexts add <name> --address ... [--default-org ...] [--log-level ...]`
* `tfc contexts use <name>`
* `tfc contexts remove <name> [--force]` (cannot remove current unless switching first)
* `tfc contexts show [<name>]` (shows resolved config)

**Plan (acceptance + verification + steps)**

* Acceptance criteria:
  * `tfc contexts list` displays all contexts with current marked
  * `tfc contexts add <name>` creates new context with required --address flag
  * `tfc contexts use <name>` switches current_context
  * `tfc contexts remove <name>` removes context (not current unless --force + switch)
  * `tfc contexts show [<name>]` displays context config (current if name omitted)
* Verification: `go test ./...`; all Gherkin scenarios pass as unit tests
* Steps:
  1. Create ContextsCmd group struct with subcommands: List, Add, Use, Remove, Show
  2. Implement ContextsListCmd - loads settings, lists contexts with marker for current
  3. Implement ContextsAddCmd - requires name and --address, optional --default-org and --log-level
  4. Implement ContextsUseCmd - updates current_context and saves
  5. Implement ContextsRemoveCmd - blocks removing current context, respects --force for confirmation
  6. Implement ContextsShowCmd - displays resolved context (defaults applied)
  7. Add helper methods to config.Settings if needed (AddContext, RemoveContext, SetCurrent)
  8. Write unit tests covering all Gherkin scenarios
  9. Wire up ContextsCmd in CLI struct

**Gherkin**

```gherkin
Feature: contexts management
  Scenario: contexts add creates a new context
    Given existing settings with context "default"
    When I run "tfc contexts add prod --address tfe.example.com --default-org acme"
    Then settings.contexts.prod.address = "tfe.example.com"
    And settings.contexts.prod.default_org = "acme"

  Scenario: contexts use switches current context
    Given settings has contexts "default" and "prod"
    When I run "tfc contexts use prod"
    Then settings.current_context = "prod"

  Scenario: remove current context is blocked
    Given current_context = "default"
    When I run "tfc contexts remove default --force"
    Then stderr contains "cannot remove current context"
    And the exit code is 2
```

**Status: DONE**

**Progress Notes**

* 2026-01-20
  * Changes:
    * Added `ContextsCmd` group struct with subcommands: List, Add, Use, Remove, Show
    * `tfc contexts list` - lists all contexts with `*` marker for current
    * `tfc contexts add <name> --ctx-address ...` - creates new context (note: flag is `--ctx-address` to avoid conflict with global `--address`)
    * `tfc contexts use <name>` - switches current_context
    * `tfc contexts remove <name>` - removes context with confirmation (blocks removing current)
    * `tfc contexts show [<name>]` - displays resolved context config
    * Created `cmd/tfc/contexts_test.go` with 13 unit tests covering all Gherkin scenarios
  * Files changed: `cmd/tfc/main.go`, `cmd/tfc/contexts_test.go`, `specs/TASKS.md`
  * Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
  * Implementation notes:
    * Used `--ctx-address` instead of `--address` for `contexts add` to avoid conflict with global `--address` flag (Kong merges flags at all levels)
    * `ContextsRemoveCmd` supports both `--force` (global) and interactive confirmation via injectable `Prompter`
    * All subcommands have injectable `baseDir` for test isolation
  * Task complete

---

### Task 06 — Terraform token discovery (env + terraformrc + credentials.tfrc.json)

**Deliverables**

* `internal/auth`:

  * Parse `address` into hostname (ignore path for token lookup)
  * Resolve env var `TF_TOKEN_<sanitized_host>` precedence
  * Parse terraform CLI config credential blocks
  * Parse `credentials.tfrc.json` produced by `terraform login` ([HashiCorp Developer][2])
  * Return `(token, source)`; never log token value

**Gherkin**

```gherkin
Feature: Terraform token discovery
  Scenario: Env token wins
    Given TF_TOKEN_app_terraform_io="env-token"
    And credentials.tfrc.json has token "file-token" for "app.terraform.io"
    When I resolve token for address "app.terraform.io"
    Then token = "env-token"
    And source = "env"

  Scenario: terraform login file token used when env missing
    Given TF_TOKEN_app_terraform_io is not set
    And credentials.tfrc.json has token "file-token" for "app.terraform.io"
    When I resolve token for address "app.terraform.io"
    Then token = "file-token"
    And source = "credentials.tfrc.json"

  Scenario: Missing token yields actionable error
    Given no token sources exist
    When I resolve token for address "app.terraform.io"
    Then error contains "no API token found"
    And error suggests running "terraform login"
```

**Status: DONE**

**Plan (acceptance + verification + steps)**

* Acceptance criteria (from PRD/Task):
  * `ResolveToken(address)` returns `(token, source, error)`
  * Precedence: env var `TF_TOKEN_<sanitized_host>` > `TF_CLI_CONFIG_FILE` credentials block > `~/.terraform.d/credentials.tfrc.json`
  * Host sanitization: `app.terraform.io` -> `app_terraform_io`
  * Address parsing: extract hostname from addresses like `app.terraform.io`, `https://tfe.example.com`, `app.terraform.io/eu`
  * Returns actionable error suggesting `terraform login` when no token found
  * Never log/print the token value
* Verification: `go test ./internal/auth/...`; all Gherkin scenarios pass as unit tests
* Steps:
  1. Create `internal/auth/token.go` with `ResolveToken(address string) (token, source string, err error)`
  2. Implement `SanitizeHost(hostname)` - replaces `.` and `-` with `_`
  3. Implement `ExtractHostname(address)` - parses URL/host to get hostname only
  4. Implement env var lookup `TF_TOKEN_<sanitized_host>`
  5. Implement Terraform CLI config parsing (`TF_CLI_CONFIG_FILE` or `~/.terraformrc` or `terraform.rc` on Windows)
  6. Implement `credentials.tfrc.json` parsing (`~/.terraform.d/credentials.tfrc.json`)
  7. Add injectable env/fs for testability (EnvGetter, FSReader interfaces)
  8. Write unit tests covering all Gherkin scenarios plus edge cases

**Progress Notes**

* 2026-01-20
  * Changes:
    * Created `internal/auth/token.go` with:
      * `TokenResolver` struct with injectable `EnvGetter` and `FSReader` interfaces
      * `ResolveToken(address)` returning `(*TokenResult, error)` with token and source
      * `ExtractHostname(address)` - extracts hostname from various address formats
      * `SanitizeHost(hostname)` - converts `.` and `-` to `_` for env var lookup
      * Token precedence: env var > TF_CLI_CONFIG_FILE > ~/.terraformrc > ~/.terraform.d/credentials.tfrc.json
      * `NoTokenError` type with actionable message suggesting `terraform login`
    * Created `internal/auth/token_test.go` with 12 unit tests covering all Gherkin scenarios:
      * Env token wins over file tokens
      * Credentials file token used when env missing
      * Missing token returns actionable error
      * Terraform CLI config parsing (.terraformrc)
      * TF_CLI_CONFIG_FILE override
      * Address with path (hostname extraction)
      * Full URL parsing
      * Precedence tests (env > config > credentials file)
  * Files changed: `internal/auth/token.go`, `internal/auth/token_test.go`, `specs/TASKS.md`
  * Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
  * Test results: `ok github.com/richclement/tfccli/internal/auth 0.360s`
  * Task complete

---

### Task 07 — Logging (logr/stdr) + settings-driven level + `--debug`

**Deliverables**

* Logger factory:

  * Reads `log_level` from current context
  * `--debug` overrides to debug
* Consistent structured fields: `request_id`, `method`, `path`, `status`, `attempt`

**Gherkin**

```gherkin
Feature: Logging level resolution
  Scenario: debug flag overrides settings
    Given settings log_level = "info"
    When I run "tfc --debug doctor"
    Then effective log level = "debug"

  Scenario: settings log level is respected
    Given settings log_level = "error"
    When I run "tfc doctor"
    Then logs do not include info-level messages
```

---

### Task 08 — Interactive confirmation + `--force`

**Deliverables**

* `internal/ui`:

  * `Confirm(prompt) (bool, error)` injectable for tests
* Enforcement:

  * All destructive ops call confirm unless `--force`

**Gherkin**

```gherkin
Feature: Confirmation workflow
  Scenario: destructive command prompts without --force
    Given a command "organizations delete org-1"
    When user answers "no"
    Then no DELETE request is sent
    And exit code is 0

  Scenario: destructive command does not prompt with --force
    Given a command "organizations delete org-1 --force"
    When I run the command
    Then no prompt is shown
```

---

### Task 09 — Output selection (TTY detection) + JSON emitter

**Deliverables**

* `internal/output`:

  * TTY detection injectable for tests
  * Default output-format: json when not TTY, else table
  * JSON emitter prints:

    * raw JSON:API body for most responses
    * `{"meta":{"status":204}}` for empty-body successes

**Gherkin**

```gherkin
Feature: Output format defaults
  Scenario: Defaults to table on TTY
    Given stdout is a TTY
    When I run "tfc doctor"
    Then effective output format = "table"

  Scenario: Defaults to json when stdout is not a TTY
    Given stdout is not a TTY
    When I run "tfc doctor"
    Then effective output format = "json"

  Scenario: Empty-body success emits meta JSON in json mode
    Given stdout is not a TTY
    And API returns 204 No Content
    When I run a delete command with --force
    Then stdout parses as JSON
    And stdout.meta.status = 204
```

---

### Task 10 — Table renderer with termenv

**Deliverables**

* Table rendering helper (deterministic columns, stable ordering)
* No styling when not TTY
* Basic PASS/WARN/FAIL formatting for doctor

**Gherkin**

```gherkin
Feature: Table rendering
  Scenario: Table output is deterministic
    Given stdout is a TTY
    When I render a table with rows in input order A,B
    Then stdout contains rows in order A,B

  Scenario: No ANSI styling when stdout is not TTY
    Given stdout is not a TTY
    When I run a table command explicitly "--output-format=table"
    Then stdout does not contain "\u001b["
```

---

### Task 11 — API base address handling + go-tfe client wiring

**Deliverables**

* Address normalization:

  * Accept `app.terraform.io`, `https://app.terraform.io`, `app.terraform.io/eu`, etc.
  * Construct API base as `<address>/api/v2`
* go-tfe client created using normalized base + token

**Gherkin**

```gherkin
Feature: Address normalization
  Scenario: Bare host becomes https URL
    When I normalize address "app.terraform.io"
    Then base URL is "https://app.terraform.io"

  Scenario: Host with path preserves path
    When I normalize address "app.terraform.io/eu"
    Then base URL is "https://app.terraform.io/eu"
    And API base is "https://app.terraform.io/eu/api/v2"
```

---

### Task 12 — HTTP middleware: retries/backoff + rate limiting + JSON:API error mapping

**Deliverables**

* Retry on 429 and transient 5xx (bounded attempts)
* Honor Retry-After when present
* Rate limiter (token bucket) to smooth bursts
* JSON:API error object decoding into typed error (`APIError{Status, Title, Detail, Errors[]}`)

**Gherkin**

```gherkin
Feature: Retry and backoff
  Scenario: Retries on 429 with Retry-After then succeeds
    Given API responds 429 with Retry-After "1" then 200
    When I run a list command
    Then API is called 2 times
    And exit code is 0

  Scenario: Stops retrying after max attempts
    Given API always responds 503
    When I run a list command
    Then API is called N times where N = max_attempts
    And stderr contains "service unavailable"
    And exit code is 2

Feature: JSON:API error mapping
  Scenario: 401 returns decoded error detail
    Given API responds 401 with JSON:API error body containing "Unauthorized"
    When I run any command
    Then stderr contains "Unauthorized"
    And exit code is 2
```

---

### Task 13 — Auto-pagination aggregator (list endpoints)

**Deliverables**

* List helper that fetches all pages and aggregates `data[]` into one JSON:API output document
* Supports `--page-size` override (default 100), but always fetch all pages

**Gherkin**

```gherkin
Feature: Auto pagination
  Scenario: Aggregates multiple pages into one JSON output
    Given API returns page 1 with 2 items and page 2 with 1 item and then no more
    When I run "tfc organizations list --output-format=json"
    Then stdout.data has length 3

  Scenario: Stops on empty page to avoid infinite loops
    Given API returns page 1 with items and page 2 empty
    When I run a list command
    Then requests stop after the empty page
```

---

### Task 14 — Subcommand: `tfc doctor`

**Deliverables**

* Checks:

  * settings exists + valid + context found
  * address parses + shows derived hostname
  * token resolved (source shown; token never printed)
  * connectivity check (simple GET, e.g., organizations list page 1)
* Output table/json

**Gherkin**

```gherkin
Feature: doctor command
  Scenario: Doctor fails when settings missing
    Given no settings.json exists
    When I run "tfc doctor"
    Then stderr contains "tfc init"
    And exit code is 2

  Scenario: Doctor reports token source
    Given env token exists for host
    And API connectivity is OK
    When I run "tfc doctor --output-format=json"
    Then stdout.checks.token.status = "pass"
    And stdout.checks.token.source = "env"

  Scenario: Doctor fails on connectivity error
    Given token exists
    And API responds 500
    When I run "tfc doctor"
    Then exit code is 2
    And output indicates connectivity failure
```

---

## Phase 1 — Resource subcommands (each is its own task)

### Task 15 — Subcommand: `tfc organizations` (CRUD)

**Deliverables**

* `organizations list`
* `organizations get <org-id>`
* `organizations create --name <name> [--email ...] ...` (minimal flags + optional `--payload-file`)
* `organizations update <org-id> ...`
* `organizations delete <org-id> [--force]` ([HashiCorp Developer][3])

**Gherkin**

```gherkin
Feature: organizations list/get
  Scenario: List calls organizations endpoint and paginates
    Given a fake API that returns 2 pages
    When I run "tfc organizations list --output-format=json"
    Then the server receives "GET /api/v2/organizations"
    And stdout.data length equals total items across pages

  Scenario: Get uses org id
    Given a fake API server
    When I run "tfc organizations get org-123 --output-format=json"
    Then the server receives "GET /api/v2/organizations/org-123"

Feature: organizations delete safety
  Scenario: Delete prompts without --force
    When I run "tfc organizations delete org-123" and answer "no"
    Then no DELETE request is sent

  Scenario: Delete sends request with --force
    Given API returns 204
    When I run "tfc organizations delete org-123 --force --output-format=json"
    Then the server receives "DELETE /api/v2/organizations/org-123"
    And stdout.meta.status = 204
```

---

### Task 16 — Subcommand: `tfc projects` (CRUD, org-scoped for list/create)

**Deliverables**

* `projects list [--org <org>]` (uses default_org if not passed)
* `projects get <project-id>`
* `projects create --org <org> --name <name> ...`
* `projects update <project-id> ...`
* `projects delete <project-id> [--force]`

**Gherkin**

```gherkin
Feature: projects org resolution
  Scenario: List uses default_org when --org not provided
    Given settings default_org = "acme"
    When I run "tfc projects list --output-format=json"
    Then the server receives "GET /api/v2/organizations/acme/projects"

  Scenario: List fails when no org available
    Given settings default_org is empty
    When I run "tfc projects list"
    Then stderr contains "organization is required"
    And exit code is 1

Feature: projects CRUD by id
  Scenario: Get uses project id endpoint
    When I run "tfc projects get prj-1 --output-format=json"
    Then the server receives "GET /api/v2/projects/prj-1"

  Scenario: Delete requires confirmation unless forced
    When I run "tfc projects delete prj-1" and answer "no"
    Then no DELETE request is sent
```

---

### Task 17 — Subcommand: `tfc workspaces` (CRUD, org-scoped for list/create)

**Deliverables**

* `workspaces list [--org <org>]`
* `workspaces get <workspace-id>`
* `workspaces create --org <org> --name <name> [--project-id <id>] ...`
* `workspaces update <workspace-id> ...`
* `workspaces delete <workspace-id> [--force]`

**Gherkin**

```gherkin
Feature: workspaces list/create org handling
  Scenario: List uses default_org
    Given settings default_org = "acme"
    When I run "tfc workspaces list --output-format=json"
    Then the server receives "GET /api/v2/organizations/acme/workspaces"

  Scenario: Create requires org (default or flag)
    Given settings default_org is empty
    When I run "tfc workspaces create --name prod"
    Then stderr contains "organization is required"
    And exit code is 1

Feature: workspaces update/delete by id
  Scenario: Update workspace sends PATCH by id
    When I run "tfc workspaces update ws-1 --output-format=json --payload-file payload.json"
    Then the server receives "PATCH /api/v2/workspaces/ws-1"

  Scenario: Delete prompts unless forced
    When I run "tfc workspaces delete ws-1" and answer "yes"
    Then the server receives "DELETE /api/v2/workspaces/ws-1"
```

---

### Task 18 — Subcommand: `tfc workspace-variables` (CRUD)

**Deliverables**

* `workspace-variables list --workspace-id <ws-id>`
* `workspace-variables create --workspace-id <ws-id> --key ... --value ... --category env|terraform [--sensitive] [--hcl] ...`
* `workspace-variables update <var-id> ...`
* `workspace-variables delete <var-id> [--force]`

**Gherkin**

```gherkin
Feature: workspace variables
  Scenario: List variables requires workspace-id
    When I run "tfc workspace-variables list"
    Then stderr contains "--workspace-id is required"
    And exit code is 1

  Scenario: Create variable posts to workspace vars endpoint
    When I run "tfc workspace-variables create --workspace-id ws-1 --key FOO --value bar --category env --output-format=json"
    Then the server receives "POST /api/v2/workspaces/ws-1/vars"
    And the request body JSON has data.type = "vars"

  Scenario: Sensitive variable does not echo value in logs
    Given --debug is enabled
    When I create a sensitive variable
    Then logs do not contain the literal variable value

  Scenario: Delete requires confirmation unless --force
    When I run "tfc workspace-variables delete var-1" and answer "no"
    Then no DELETE request is sent
```

---

### Task 19 — Subcommand: `tfc workspace-resources` (read-only list)

**Deliverables**

* `workspace-resources list --workspace-id <ws-id>`
* Table columns: resource id/type/name/provider (where available)

**Gherkin**

```gherkin
Feature: workspace resources
  Scenario: List resources hits correct endpoint
    When I run "tfc workspace-resources list --workspace-id ws-1 --output-format=json"
    Then the server receives "GET /api/v2/workspaces/ws-1/resources"

  Scenario: Output JSON is raw JSON:API
    When I run "tfc workspace-resources list --workspace-id ws-1 --output-format=json"
    Then stdout has a top-level "data" field
```

---

### Task 20 — Subcommand: `tfc runs` (list/get/create + actions)

**Deliverables**

* `runs list --workspace-id <ws-id>` (start here; add org-scoped list later if desired)
* `runs get <run-id>`
* `runs create --workspace-id <ws-id> --configuration-version-id <cv-id> [--message ...]`
* Actions (confirm/--force):

  * `runs apply <run-id>`
  * `runs discard <run-id>`
  * `runs cancel <run-id>`
  * `runs force-cancel <run-id>`

**Gherkin**

```gherkin
Feature: runs create and actions
  Scenario: Create run requires workspace-id and configuration-version-id
    When I run "tfc runs create --workspace-id ws-1"
    Then stderr contains "--configuration-version-id is required"
    And exit code is 1

  Scenario: Create run posts correct payload
    When I run "tfc runs create --workspace-id ws-1 --configuration-version-id cv-1 --output-format=json"
    Then the server receives "POST /api/v2/runs"
    And request body JSON has data.type = "runs"

  Scenario: Apply prompts unless forced
    When I run "tfc runs apply run-1" and answer "no"
    Then no POST request is sent

  Scenario: Apply posts to action endpoint with --force
    When I run "tfc runs apply run-1 --force"
    Then the server receives "POST /api/v2/runs/run-1/actions/apply"
```

---

### Task 21 — Subcommand: `tfc plans` (get + downloads via 307 redirect)

**Deliverables**

* `plans get <plan-id>`
* `plans json-output <plan-id> [--out <file>]` (follows 307) ([HashiCorp Developer][1])
* `plans sanitized-plan <plan-id> [--out <file>]` (follows 307) ([HashiCorp Developer][1])
* Redirect security: do not forward `Authorization` header to redirected host

**Gherkin**

```gherkin
Feature: plans read and download
  Scenario: Get plan uses plan id endpoint
    When I run "tfc plans get plan-1 --output-format=json"
    Then the server receives "GET /api/v2/plans/plan-1"

  Scenario: json-output follows 307 and writes to stdout when --out not set
    Given API responds 307 with Location "https://archivist.example/plan.json"
    And GET to the Location returns body "{ \"format_version\": \"1.0\" }"
    When I run "tfc plans json-output plan-1"
    Then stdout equals "{ \"format_version\": \"1.0\" }"

  Scenario: Redirect follow does not forward Authorization header
    Given the first request includes Authorization
    When the client follows Location to "https://archivist.example/plan.json"
    Then the second request does not include an Authorization header

  Scenario: json-output with --out writes file and emits meta in json mode
    Given stdout is not a TTY
    When I run "tfc plans json-output plan-1 --out out.json"
    Then "out.json" exists
    And stdout.meta.written_to = "out.json"
```

---

### Task 22 — Subcommand: `tfc applies` (get + errored-state download via 307)

**Deliverables**

* `applies get <apply-id>`
* `applies errored-state <apply-id> [--out <file>]` (follows 307) ([HashiCorp Developer][4])
* Redirect security: no token forwarded on Location fetch

**Gherkin**

```gherkin
Feature: applies read and errored state recovery
  Scenario: Get apply uses apply id endpoint
    When I run "tfc applies get apply-1 --output-format=json"
    Then the server receives "GET /api/v2/applies/apply-1"

  Scenario: errored-state follows 307 and writes to file
    Given API responds 307 with Location "https://archivist.example/errored.tfstate"
    And GET to the Location returns bytes "STATEBYTES"
    When I run "tfc applies errored-state apply-1 --out errored.tfstate"
    Then file "errored.tfstate" contains "STATEBYTES"

  Scenario: Redirect follow does not forward Authorization header
    When the client follows the Location URL
    Then the request to archivist has no Authorization header
```

---

### Task 23 — Subcommand: `tfc configuration-versions` (create/list/get/upload/download/archive)

**Deliverables**

* `configuration-versions list --workspace-id <ws-id>`
* `configuration-versions get <cv-id>`
* `configuration-versions create --workspace-id <ws-id> [--auto-queue-runs=true|false]`
* `configuration-versions upload <cv-id> --file <path>` (uses upload URL, no auth header)
* `configuration-versions download <cv-id> [--out <file>]` (follow redirect if used)
* `configuration-versions archive <cv-id> [--force]`

**Gherkin**

```gherkin
Feature: configuration versions lifecycle
  Scenario: Create configuration version requires workspace-id
    When I run "tfc configuration-versions create"
    Then stderr contains "--workspace-id is required"
    And exit code is 1

  Scenario: Upload uses upload-url and does not attach Authorization
    Given create/get returns upload-url "https://archivist.example/upload/abc"
    When I run "tfc configuration-versions upload cv-1 --file ./cfg.tar.gz"
    Then a PUT is sent to "https://archivist.example/upload/abc"
    And the PUT request has no Authorization header

  Scenario: Archive requires confirmation unless forced
    When I run "tfc configuration-versions archive cv-1" and answer "no"
    Then no PATCH/POST request is sent
```

---

### Task 24 — Subcommand: `tfc users` (get)

**Deliverables**

* `users get <user-id>` (raw JSON:API doc) ([HashiCorp Developer][5])
* (Optional but useful): `users tokens list/create/delete` later; v1 can be just `get`

**Gherkin**

```gherkin
Feature: users get
  Scenario: Get user calls /users/:id
    When I run "tfc users get user-1 --output-format=json"
    Then the server receives "GET /api/v2/users/user-1"

  Scenario: 404 is surfaced clearly
    Given API responds 404 with JSON:API error "User not found"
    When I run "tfc users get user-404"
    Then stderr contains "User not found"
    And exit code is 2
```

---

### Task 25 — Subcommand: `tfc invoices` (list/get/next)

**Deliverables**

* `invoices list [--org <org>]` (uses default_org if not passed)
* `invoices get <invoice-id>` (if supported by API; otherwise omit)
* `invoices next [--org <org>]` (explicitly supported) ([HashiCorp Developer][6])
* Friendly error if invoices API not available for account/org (Cloud-only note) ([GitHub][7])

**Gherkin**

```gherkin
Feature: invoices
  Scenario: Next invoice uses org-scoped endpoint
    Given settings default_org = "acme"
    When I run "tfc invoices next --output-format=json"
    Then the server receives "GET /api/v2/organizations/acme/invoices/next"

  Scenario: List invoices requires org when no default org
    Given settings default_org is empty
    When I run "tfc invoices list"
    Then stderr contains "organization is required"
    And exit code is 1

  Scenario: API not available returns actionable error
    Given API responds 404 or error indicating invoices unavailable
    When I run "tfc invoices next --org acme"
    Then stderr mentions "invoices API is only available in HCP Terraform"
```

---

## Phase 2 — Cross-cutting quality and release readiness

### Task 26 — `tfc version` + goreleaser config (skip Homebrew wiring)

**Deliverables**

* `tfc version` prints:

  * version (semver or dev)
  * commit SHA
  * build date
* `goreleaser.yml` builds `tfc` for major OS/arch (no brew tap)

**Gherkin**

```gherkin
Feature: version command
  Scenario: Version prints build metadata
    When I run "tfc version"
    Then stdout contains "version"
    And stdout contains "commit"
    And stdout contains "date"

  Scenario: Version command exits successfully
    When I run "tfc version"
    Then exit code is 0
```

---

### Task 27 — README + command examples + agent-friendly usage notes

**Deliverables**

* README includes:

  * install/build instructions
  * auth discovery explanation (env, terraformrc, credentials.tfrc.json) ([HashiCorp Developer][2])
  * examples for each subcommand (including redirect downloads and `--out`)
  * context usage examples

**Gherkin**

```gherkin
Feature: Documentation presence
  Scenario: README exists and mentions init and contexts
    Given the repository source tree
    When I read "README.md"
    Then it contains "tfc init"
    And it contains "tfc contexts"
```

---

### Task 28 — Test harness utilities (shared across command tests)

**Deliverables**

* `internal/testutil`:

  * temp-home helper that rewrites HOME for tests
  * fake TTY detector
  * fake prompter (scripted answers)
  * httptest server request recorder:

    * captures method/path/query/headers/body
* Golden fixture approach for JSON:API examples (optional)

**Gherkin**

```gherkin
Feature: Test harness
  Scenario: Request recorder captures headers and body
    Given a fake API server with recorder
    When I run a command that POSTs JSON
    Then the recorder stores method, path, headers, and body bytes

  Scenario: Temp home isolates settings file
    Given a temp home directory
    When a test writes "~/.tfccli/settings.json"
    Then the real user home is not modified
```

---

If you want, I can also produce a **canonical flag spec** per command (exact flag names + which fields map into the JSON:API request body for create/update), but the above is already implementable with a pragmatic approach: **minimal typed flags** + an optional `--payload-file` escape hatch for anything not yet modeled.

[1]: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/plans?utm_source=chatgpt.com "/plans API reference for HCP Terraform - HashiCorp Developer"
[2]: https://developer.hashicorp.com/terraform/cli/commands/login?utm_source=chatgpt.com "terraform login command reference - HashiCorp Developer"
[3]: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/organizations?utm_source=chatgpt.com "/organizations API reference for HCP Terraform | Terraform | HashiCorp ..."
[4]: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/applies?utm_source=chatgpt.com "/applies API reference for HCP Terraform - HashiCorp Developer"
[5]: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/users?utm_source=chatgpt.com "/users API reference for HCP Terraform - HashiCorp Developer"
[6]: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/invoices?utm_source=chatgpt.com "/invoices API reference for HCP Terraform - HashiCorp Developer"
[7]: https://github.com/hashicorp/terraform-docs-common/blob/main/website/docs/cloud-docs/api-docs/invoices.mdx?utm_source=chatgpt.com "terraform-docs-common/website/docs/cloud-docs/api-docs/invoices.mdx at ..."
