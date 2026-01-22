# Architectural Review: tfccli

**Review Date:** 2026-01-22
**Reviewer Role:** Senior Architect
**Scope:** Full codebase review focusing on architecture, design, dependency boundaries, modularity, and complexity

---

## Executive Summary

The tfccli codebase is well-structured with clean layered architecture and no circular dependencies. Test coverage is excellent (~68% of codebase is tests). However, significant code duplication has accumulated as features were added, creating maintenance burden and inconsistency risks. The primary issues are repeated boilerplate patterns that should be consolidated into shared abstractions.

---

## Findings

### 1. Massive Code Duplication: `resolveXxxClientConfig` Functions

**Severity:** High
**Location:** Multiple files in `cmd/tfc/`
**Impact:** Maintenance burden, inconsistency risk

Nearly identical configuration resolution functions exist across multiple files:

| File | Function | Return Type |
|------|----------|-------------|
| `common.go:16` | `resolveClientConfig` | `(cfg, org, error)` |
| `organizations.go:75` | `resolveOrgsClientConfig` | `(cfg, error)` |
| `runs.go:122` | `resolveRunsClientConfig` | `(cfg, error)` |
| `applies.go:125` | `resolveAppliesClientConfig` | `(cfg, error)` |
| `users.go:127` | `resolveUsersClientConfig` | `(cfg, error)` |
| `invoices.go:224` | `resolveInvoicesClientConfig` | `(cfg, org, error)` |

Each function contains the same ~30 lines: load settings, resolve context name, get context, apply defaults, override address, resolve token. The only variation is whether `org` is included in the return.

**Remediation:** Consolidate into a single function in `common.go` that always returns `(cfg, org, error)`. Commands that don't need `org` can use `_` to discard it. Remove the five duplicate functions.

---

### 2. Large `main.go` File (734 Lines)

**Severity:** Medium
**Location:** `cmd/tfc/main.go`
**Impact:** Reduced navigability, harder to maintain

The main.go file contains multiple command implementations that should be in separate files:
- `DoctorCmd` and related types (lines 67-252)
- `InitCmd` (lines 254-371)
- `ContextsCmd` and all subcommands (lines 373-671)
- CLI struct and run() function (remaining)

**Remediation:** Split into focused files:
- `main.go` - CLI struct, run(), versionString(), exitCodeForError()
- `doctor.go` - DoctorCmd and DoctorCheck/DoctorResult types
- `init.go` - InitCmd
- `contexts.go` - ContextsCmd and all Contexts subcommands

---

### 3. Unused Logging Infrastructure

**Severity:** Medium
**Location:** `internal/logging/logger.go`
**Impact:** Dead code, wasted implementation effort

The `internal/logging` package provides a complete logr-based logger factory with:
- Level-based logging (debug, info, warn, error)
- `--debug` flag override support
- Configurable output writer

However, **no command in `cmd/tfc/` imports or uses this package**. The CLI has a `--debug` flag (line 47 of main.go) but it's never connected to actual logging.

**Remediation:** Either:
- A) Integrate logging into commands (add request/response logging, timing, debug output)
- B) Remove the logging package if logging is not needed

---

### 4. Boilerplate Client Interface Pattern

**Severity:** Medium
**Location:** All resource command files
**Impact:** ~60-80 lines of boilerplate per resource

Every resource follows this pattern:

```go
// Interface
type xxxClient interface { ... }

// Factory type
type xxxClientFactory func(cfg tfcapi.ClientConfig) (xxxClient, error)

// Real implementation wrapping tfe.Client
type realXxxClient struct { client *tfe.Client }

// Method implementations delegating to client.Xxx.Method()
func (c *realXxxClient) List(...) { return c.client.Xxx.List(...) }

// Factory function
func defaultXxxClientFactory(cfg tfcapi.ClientConfig) (xxxClient, error) {
    client, err := tfcapi.NewClient(cfg)
    return &realXxxClient{client: client}, nil
}
```

This appears in: organizations.go, workspaces.go, projects.go, runs.go, plans.go, applies.go, configuration_versions.go, workspace_variables.go, workspace_resources.go, users.go, invoices.go (11 times).

**Remediation:** Consider using generics (Go 1.18+) or a code generator to reduce boilerplate. At minimum, document this as an intentional pattern for testability.

---

### 5. Empty `doc.go` Files

**Severity:** Low
**Location:** `internal/*/doc.go` (6 files)
**Impact:** Missed documentation opportunity

All doc.go files contain only package declarations with no documentation:

```go
package auth
```

**Remediation:** Add package-level documentation describing each package's purpose:
- `internal/auth` - Token discovery following Terraform CLI conventions
- `internal/cmd` - Shared command utilities (RuntimeError type)
- `internal/config` - Settings schema and persistence
- `internal/output` - Output formatting with TTY awareness
- `internal/tfcapi` - TFC API client wrapper and utilities
- `internal/ui` - User interaction prompts

---

### 6. Inconsistent TTY Detection Pattern

**Severity:** Medium
**Location:** All command files
**Impact:** Code duplication, inconsistency risk

Two patterns are used interchangeably:

**Pattern A (using helper):**
```go
// common.go helper
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

**Pattern B (inline):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

Pattern B appears in: organizations.go (lines 152-156, 227-231, etc.), users.go (206-210), invoices.go (316-320), applies.go (204-208).

**Remediation:** Use `resolveFormat()` helper consistently in all commands. Update the helper if it doesn't meet all use cases.

---

### 7. Duplicated HTTP Client Code

**Severity:** Medium
**Location:** `cmd/tfc/users.go`, `cmd/tfc/invoices.go`
**Impact:** Duplicated error handling, inconsistent behavior

Both files implement custom HTTP clients because go-tfe doesn't cover these endpoints. They share similar:
- Request construction with Authorization header
- JSON:API error parsing
- Status code handling

For example, error parsing in users.go:100 and invoices.go:188-196 are nearly identical.

**Remediation:** Extract a shared HTTP client helper in `internal/tfcapi/` that handles:
- Request construction with auth headers
- JSON:API error parsing
- Common status code handling (401, 403, 404)

---

### 8. Inconsistent API Error Handling Format

**Severity:** Low
**Location:** Throughout `cmd/tfc/` files
**Impact:** Inconsistent error message formatting

Two patterns are used:

**Pattern A (using %w):**
```go
return internalcmd.NewRuntimeError(fmt.Errorf("failed to list: %w", apiErr))
```

**Pattern B (using %s):**
```go
return internalcmd.NewRuntimeError(fmt.Errorf("failed to list: %s", apiErr.Error()))
```

Pattern A is used in organizations.go. Pattern B is used in runs.go, users.go, and elsewhere.

**Remediation:** Standardize on Pattern A (`%w`) to preserve error chain for callers who want to inspect the underlying error with `errors.As()`.

---

### 9. Verbose Dependency Injection Pattern

**Severity:** Medium
**Location:** All command structs
**Impact:** Boilerplate in every Run() method

Every command struct has 5-6 injectable dependencies:

```go
type XxxCmd struct {
    // CLI args...

    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        io.Writer
    clientFactory xxxClientFactory
    prompter      ui.Prompter  // some commands
}
```

And every Run() method starts with nil checks:

```go
func (c *XxxCmd) Run(cli *CLI) error {
    if c.ttyDetector == nil {
        c.ttyDetector = &output.RealTTYDetector{}
    }
    if c.stdout == nil {
        c.stdout = os.Stdout
    }
    if c.clientFactory == nil {
        c.clientFactory = defaultXxxClientFactory
    }
    // ... maybe more
```

**Remediation:** Consider a "command context" struct that bundles common dependencies with sensible defaults:

```go
type CmdContext struct {
    Stdout        io.Writer
    TTYDetector   output.TTYDetector
    TokenResolver *auth.TokenResolver
    BaseDir       string
}

func DefaultCmdContext() *CmdContext { ... }
```

---

### 10. Missing Abstraction for Org-Required Commands

**Severity:** Low
**Location:** `invoices.go`, likely others
**Impact:** Duplicated validation logic

Commands that require an organization follow this pattern:

```go
cfg, org, err := resolveInvoicesClientConfig(cli, c.baseDir, c.tokenResolver)
if err != nil {
    return internalcmd.NewRuntimeError(err)
}
if org == "" {
    return fmt.Errorf("organization is required: use --org flag or set default_org in context")
}
```

**Remediation:** Add a helper `resolveClientConfigWithRequiredOrg()` that returns an error if org is empty after resolution.

---

### 11. No Context Propagation

**Severity:** Medium
**Location:** All command Run() methods
**Impact:** No support for cancellation, timeouts

Every command creates `context.Background()`:

```go
ctx := context.Background()
client.List(ctx, ...)
```

This means:
- User cannot cancel long-running operations (Ctrl+C)
- No timeout support for API calls
- No request tracing/correlation

**Remediation:** Accept context from the CLI framework or create a context with signal handling:

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()
```

---

### 12. Awkward `forceFlag *bool` Pattern

**Severity:** Low
**Location:** Commands with confirmation prompts
**Impact:** Test complexity, unusual API

Commands that respect `--force` use this pattern:

```go
type XxxCmd struct {
    // ...
    forceFlag *bool // Pointer to allow injection from tests
}

func (c *XxxCmd) Run(cli *CLI) error {
    force := cli.Force
    if c.forceFlag != nil {
        force = *c.forceFlag
    }
}
```

This exists in: ContextsRemoveCmd, OrganizationsDeleteCmd, RunsApplyCmd, RunsDiscardCmd, RunsCancelCmd, RunsForceCancelCmd.

**Remediation:** Instead of a pointer override, have tests set `cli.Force = true` directly. Or use a functional option pattern for test configuration.

---

### 13. No Shared HTTP Client

**Severity:** Low
**Location:** users.go, invoices.go, applies.go, configuration_versions.go
**Impact:** No connection pooling, no consistent timeout configuration

Each command that needs custom HTTP creates its own client:

```go
httpClient := &http.Client{}
```

**Remediation:** Create a shared HTTP client in `internal/tfcapi/` with appropriate defaults (timeout, keep-alive settings). This also enables easier mocking in tests.

---

### 14. Inconsistent Empty List Handling

**Severity:** Low
**Location:** List commands
**Impact:** Inconsistent UX

Different list commands handle empty results differently:

- `organizations list`: Prints "No organizations found." (line 166)
- `runs list`: Shows empty table (no special message)
- `workspaces list`: Shows empty table
- `configuration-versions list`: Shows empty table

**Remediation:** Standardize: either all list commands print a "No X found" message for empty results, or none do (show empty table consistently).

---

### 15. JSON Output Wrapping Inconsistency

**Severity:** Low
**Location:** JSON output across commands
**Impact:** API consumers must handle different response shapes

Most commands wrap in `{"data": ...}`:
```go
result := map[string]any{"data": org}
```

But users.go emits raw JSON:API response, and some meta outputs use different shapes:
```go
{"meta": {"status": "uploaded", ...}}
{"meta": {"written_to": ..., "bytes": ...}}
```

**Remediation:** Document the JSON output contract clearly. Consider a consistent envelope:
```json
{"data": ..., "meta": {...}}
```

---

## Architecture Strengths

For balance, the codebase has several strong architectural qualities:

1. **Clean dependency graph** - No circular dependencies between internal packages
2. **Excellent testability** - Dependency injection enables thorough unit testing
3. **Consistent command structure** - All commands follow the same Run(cli *CLI) pattern
4. **TTY-aware output** - Smart defaults based on terminal detection
5. **Terraform compatibility** - Token discovery follows terraform CLI conventions
6. **Multi-context support** - Easy switching between TFC/TFE instances
7. **High test coverage** - ~16,100 lines of tests, 487 test functions
8. **Clear exit code semantics** - 0=success, 1=usage error, 2=runtime error, 3=internal error

---

## Recommended Priority

| Priority | Finding | Effort | Impact |
|----------|---------|--------|--------|
| P1 | #1 Duplicate resolveClientConfig | Low | High |
| P1 | #6 Inconsistent TTY detection | Low | Medium |
| P1 | #8 Inconsistent error format | Low | Low |
| P2 | #2 Large main.go | Medium | Medium |
| P2 | #3 Unused logging | Medium | Medium |
| P2 | #7 Duplicated HTTP client | Medium | Medium |
| P2 | #11 No context propagation | Medium | Medium |
| P3 | #4 Client interface boilerplate | High | Medium |
| P3 | #5 Empty doc.go files | Low | Low |
| P3 | #9 Verbose DI pattern | High | Medium |
| P3 | #10 Org-required abstraction | Low | Low |
| P3 | #12 forceFlag pattern | Low | Low |
| P3 | #13 No shared HTTP client | Low | Low |
| P3 | #14 Empty list handling | Low | Low |
| P3 | #15 JSON output inconsistency | Medium | Low |

---

## Next Steps

1. Address P1 items first - they're quick wins with significant impact
2. Create tickets for P2 items with clear acceptance criteria
3. Discuss P3 items in architecture review before committing to changes

---

## Task Log

### Task: #1 Consolidate duplicate `resolveXxxClientConfig` functions

**Status:** DONE
**Priority:** P1

**Acceptance Criteria:**
- Single `resolveClientConfig` function in `common.go` that returns `(cfg, org, error)`
- All duplicate functions (`resolveOrgsClientConfig`, `resolveRunsClientConfig`, `resolveAppliesClientConfig`, `resolveUsersClientConfig`, `resolveInvoicesClientConfig`) removed
- All callers updated to use the common function
- Tests pass unchanged (behavior is identical)

**Verification:**
- `make fmt` passes
- `make lint` passes
- `make build` passes
- `make test` passes

**Implementation Plan:**
1. Keep `resolveClientConfig` in `common.go` (already returns cfg, org, error)
2. Update organizations.go to use `resolveClientConfig` and discard `_` for org
3. Update runs.go to use `resolveClientConfig` and discard `_` for org
4. Update applies.go to use `resolveClientConfig` and discard `_` for org
5. Update users.go to use `resolveClientConfig` and discard `_` for org
6. Update invoices.go to use `resolveClientConfig` (already needs org)
7. Remove duplicate function definitions from each file
8. Run feedback loops and verify

**Progress Notes:**

_2026-01-22:_ Completed.

Files changed:
- `cmd/tfc/organizations.go`: Removed `resolveOrgsClientConfig`, updated 5 call sites to use `resolveClientConfig` with `_` for org, removed unused `internal/config` import
- `cmd/tfc/runs.go`: Removed `resolveRunsClientConfig`, updated 7 call sites to use `resolveClientConfig` with `_` for org, removed unused `internal/config` import
- `cmd/tfc/applies.go`: Removed `resolveAppliesClientConfig`, updated 2 call sites to use `resolveClientConfig` with `_` for org, removed unused `internal/config` import
- `cmd/tfc/users.go`: Removed `resolveUsersClientConfig`, updated 1 call site to use `resolveClientConfig` with `_` for org, removed unused `internal/config` import
- `cmd/tfc/invoices.go`: Removed `resolveInvoicesClientConfig`, updated 2 call sites to use `resolveClientConfig`, removed unused `internal/config` import

Commands run:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests pass, behavior unchanged)

Net effect: Removed ~165 lines of duplicate code across 5 files. All commands now use the single `resolveClientConfig` function from `common.go`.

---

### Task: #6 Inconsistent TTY detection pattern

**Status:** DONE
**Priority:** P1

**Acceptance Criteria:**
- All commands use the `resolveFormat()` helper from `common.go` instead of inline TTY detection
- The inline pattern (`isTTY := false; if f, ok := c.stdout.(*os.File); ok { isTTY = ... }; format := output.ResolveOutputFormat(...)`) is eliminated
- Tests pass unchanged (behavior is identical)

**Verification:**
- `make fmt` passes
- `make lint` passes
- `make build` passes
- `make test` passes
- `grep -r "isTTY := false" cmd/tfc/` only shows `common.go` (the helper itself)

**Implementation Plan:**
1. Update organizations.go (5 occurrences) to use `resolveFormat()`
2. Update workspaces.go (5 occurrences) to use `resolveFormat()`
3. Update users.go (1 occurrence) to use `resolveFormat()`
4. Update applies.go (2 occurrences) to use `resolveFormat()`
5. Update invoices.go (2 occurrences) to use `resolveFormat()`
6. Update main.go (2 occurrences in ContextsListCmd, ContextsShowCmd) to use `resolveFormat()`
7. Run feedback loops and verify

**Progress Notes:**

_2026-01-22:_ Completed.

Files changed:
- `cmd/tfc/organizations.go`: Replaced 5 inline TTY detection blocks with `resolveFormat()` calls
- `cmd/tfc/workspaces.go`: Replaced 5 inline TTY detection blocks with `resolveFormat()` calls
- `cmd/tfc/users.go`: Replaced 1 inline TTY detection block with `resolveFormat()` call
- `cmd/tfc/applies.go`: Replaced 2 inline TTY detection blocks with `resolveFormat()` calls
- `cmd/tfc/invoices.go`: Replaced 2 inline TTY detection blocks with `resolveFormat()` calls
- `cmd/tfc/main.go`: Replaced 2 inline TTY detection blocks with `resolveFormat()` calls (ContextsListCmd, ContextsShowCmd)

Commands run:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all 487 test functions pass, behavior unchanged)
- `grep -r "isTTY := false" cmd/tfc/` - only shows `common.go:59` (the helper itself)

Net effect: Removed 17 inline TTY detection patterns (5 lines each = ~85 lines removed). All commands now use the consistent `resolveFormat()` helper from `common.go`.

---

### Task: #8 Inconsistent API error handling format

**Status:** DONE
**Priority:** P1

**Acceptance Criteria:**
- All API error wrapping uses `%w` verb with the error directly (e.g., `fmt.Errorf("failed to X: %w", apiErr)`)
- No instances of `%s` with `.Error()` method call (e.g., `fmt.Errorf("failed to X: %s", apiErr.Error())`)
- This preserves error chain for callers who want to inspect the underlying error with `errors.As()`
- Tests pass unchanged (behavior is identical for error messages)

**Verification:**
- `make fmt` passes
- `make lint` passes
- `make build` passes
- `make test` passes
- `grep -r "apiErr.Error()" cmd/tfc/` returns no results

**Implementation Plan:**
1. Update invoices.go (2 occurrences)
2. Update projects.go (5 occurrences)
3. Update runs.go (7 occurrences)
4. Update workspace_variables.go (5 occurrences)
5. Update workspaces.go (5 occurrences)
6. Update configuration_versions.go (6 occurrences)
7. Update plans.go (3 occurrences)
8. Update applies.go (1 occurrence)
9. Update workspace_resources.go (1 occurrence)
10. Update users.go (1 occurrence)
11. Run feedback loops and verify

**Progress Notes:**

_2026-01-22:_ Completed.

Files changed:
- `cmd/tfc/invoices.go`: Changed 2 occurrences from `%s", apiErr.Error()` to `%w", apiErr`
- `cmd/tfc/projects.go`: Changed 5 occurrences
- `cmd/tfc/runs.go`: Changed 7 occurrences
- `cmd/tfc/workspace_variables.go`: Changed 5 occurrences
- `cmd/tfc/workspaces.go`: Changed 5 occurrences
- `cmd/tfc/configuration_versions.go`: Changed 6 occurrences
- `cmd/tfc/plans.go`: Changed 3 occurrences
- `cmd/tfc/applies.go`: Changed 1 occurrence
- `cmd/tfc/workspace_resources.go`: Changed 1 occurrence
- `cmd/tfc/users.go`: Changed 1 occurrence

Commands run:
- `make fmt` - passed
- `make lint` - passed (cache permission warnings only)
- `make build` - passed
- `make test` - passed (all tests pass)
- `grep -r "apiErr.Error()" cmd/tfc/` - no matches found (verified all replaced)

Net effect: Changed 36 error wrapping statements from `%s` with `.Error()` to `%w` with the error directly. This preserves the error chain for callers using `errors.As()` to inspect underlying errors.

---

### Task: #2 Large main.go file (734 lines)

**Status:** DONE
**Priority:** P2

**Acceptance Criteria:**
- `main.go` contains only: CLI struct, run(), versionString(), exitCodeForError(), main()
- `doctor.go` contains DoctorCmd, DoctorCheck, DoctorResult types and methods
- `init.go` contains InitCmd and Run method
- `contexts.go` contains ContextsCmd and all Contexts subcommands
- All tests pass unchanged (behavior is identical)
- No circular imports

**Verification:**
- `make fmt` passes
- `make lint` passes
- `make build` passes
- `make test` passes

**Implementation Plan:**
1. Create `doctor.go` with DoctorCmd, DoctorCheck, DoctorResult, doctorClient interface, defaultClientFactory, outputAndError method
2. Create `init.go` with InitCmd and Run method
3. Create `contexts.go` with ContextsCmd and all subcommands (List, Add, Use, Remove, Show)
4. Remove extracted code from `main.go`, keeping only CLI struct, version handling, run(), exitCodeForError(), printParseError(), main()
5. Run feedback loops and verify

**Progress Notes:**

_2026-01-22:_ Completed.

Files created:
- `cmd/tfc/doctor.go` (202 lines): DoctorCmd, DoctorCheck, DoctorResult types, doctorClient interface, Run() method, outputAndError() helper, defaultDoctorClientFactory()
- `cmd/tfc/init.go` (132 lines): InitCmd type and Run() method
- `cmd/tfc/contexts.go` (305 lines): ContextsCmd, ContextsListCmd, ContextsAddCmd, ContextsUseCmd, ContextsRemoveCmd, ContextsShowCmd types and all Run() methods, contextListItem and contextShowItem types

Files changed:
- `cmd/tfc/main.go`: Reduced from 724 lines to 119 lines. Now contains only: CLI struct, version handling, main(), run(), printParseError(), exitCodeForError()

Commands run:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests pass, behavior unchanged)

Net effect: Reduced main.go from 724 lines to 119 lines by extracting DoctorCmd (202 lines), InitCmd (132 lines), and ContextsCmd with subcommands (305 lines) into focused files. No circular imports, all tests pass unchanged.
