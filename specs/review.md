# Architectural Review: tfccli

**Review Date:** 2026-01-22
**Reviewer Role:** Senior Architect
**Scope:** Full codebase review focusing on architecture, design, dependency boundaries, modularity, and complexity

---

## Executive Summary

The tfccli codebase is well-structured with clean layered architecture and no circular dependencies. Test coverage is excellent (~68% of codebase is tests). However, significant code duplication has accumulated as features were added, creating maintenance burden and inconsistency risks. The primary issues are repeated boilerplate patterns that should be consolidated into shared abstractions.

---

## Tasks

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

| Priority | Task | Effort | Impact |
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

~~1. Address P1 items first - they're quick wins with significant impact~~ ✓ DONE
~~2. Create tickets for P2 items with clear acceptance criteria~~ ✓ DONE
3. Discuss P3 items in architecture review before committing to changes

---

## Task Log

### 2026-01-22 - Status Update

**Analysis complete. The following tasks have been resolved:**

| Task | Status | Notes |
|------|--------|-------|
| #1 Duplicate resolveClientConfig | DONE | Single function in `common.go` used by all commands |
| #2 Large main.go | DONE | Split into focused files, now 151 lines |
| #3 Unused logging | DONE | Logger created in main.go, used in common.go for debug output |
| #5 Empty doc.go | DONE | All doc.go files have package documentation |
| #6 Inconsistent TTY detection | DONE | All commands use `resolveFormat()` helper |
| #8 Inconsistent error format | DONE | All error handling uses `%w` for error wrapping |
| #11 No context propagation | DONE | `cmdContext()` helper and signal handling in main.go |

**All P1 and P2 tasks are now complete.**

**P3 Tasks (lower priority - future work):**

| Task | Status | Priority | Notes |
|------|--------|----------|-------|
| #4 Client interface boilerplate | TODO | P3 | Consider generics or code generator |
| #9 Verbose DI pattern | TODO | P3 | Consider CmdContext struct |
| #10 Org-required abstraction | DONE | P3 | Add helper for required org |
| #12 forceFlag pattern | TODO | P3 | Use cli.Force directly in tests |
| #14 Empty list handling | DONE | P3 | Standardize empty result messages |
| #15 JSON output inconsistency | TODO | P3 | Document JSON output contract |

---

### Task #7: Duplicated HTTP Client Code

**Status:** DONE (No Action Needed)

**Analysis (2026-01-22):**

Upon code review, the direct `http.Client{}` usage in cmd/tfc files is **intentional and correct** per PRD Section 10:

> "Do not forward the Terraform API token (Authorization header) to the redirected host."

The remaining `http.Client{}` usages serve security-compliant operations:

| File | Function | Purpose |
|------|----------|---------|
| `users.go` | - | Now uses `tfcapi.HTTPClient` for API calls ✓ |
| `plans.go:259` | `defaultDownloadClient` | Downloads sanitized plan from S3 (no auth header) |
| `configuration_versions.go:413` | `defaultUploadClient` | Uploads to S3 presigned URL (no auth header) |
| `applies.go:278` | `defaultDownloadClient` | Downloads errored state from S3 (no auth header) |
| `invoices.go:220` | client init | Cursor-based pagination + special error handling; uses `tfcapi.ParseJSONAPIErrorResponse` |

**Conclusion:** Not duplication - these are correct implementations of PRD security requirements. Blob storage operations MUST NOT include Authorization headers.

---

### Task #10: Org-Required Abstraction

**Status:** DONE

**Acceptance Criteria:**
- Add `resolveClientConfigWithRequiredOrg()` helper in `common.go` that returns an error if org is empty after resolution
- Error should use plain `fmt.Errorf` (exit code 1 per PRD: "CLI usage / validation errors")
- Consistent error message format across all commands
- Replace duplicated validation in projects.go, workspaces.go, invoices.go

**Verification:**
- `make test` passes
- Commands that require org return exit code 1 (not 2) when org missing

**Implementation Plan:**
1. Add `resolveClientConfigWithRequiredOrg()` in `common.go`
2. Update projects.go to use the new helper (list, create)
3. Update workspaces.go to use the new helper (list, create)
4. Update invoices.go to use the new helper (list, next)
5. Verify tests pass
6. Run feedback loops

**Progress Notes (2026-01-23):**

Completed implementation of org-required abstraction helper.

**Files Changed:**
- `cmd/tfc/common.go` - Added `errOrgRequired` sentinel error and `resolveClientConfigWithRequiredOrg()` helper
- `cmd/tfc/projects.go` - Updated `ProjectsListCmd.Run()` and `ProjectsCreateCmd.Run()` to use new helper
- `cmd/tfc/workspaces.go` - Updated `WorkspacesListCmd.Run()` and `WorkspacesCreateCmd.Run()` to use new helper
- `cmd/tfc/invoices.go` - Updated `InvoicesListCmd.Run()` and `InvoicesNextCmd.Run()` to use new helper
- `cmd/tfc/projects_test.go` - Fixed tests to expect exit code 1 (not 2) per TASKS.md spec
- `cmd/tfc/workspaces_test.go` - Fixed tests to expect exit code 1 (not 2) per TASKS.md spec

**Bug Fix:**
Projects and workspaces commands were incorrectly returning exit code 2 (RuntimeError) when org was missing, but the PRD (section 6) and TASKS.md explicitly state this should be exit code 1 (usage error). The helper now returns a plain error which maps to exit code 1.

**Commands Run:**
- `make fmt` - success
- `make lint` - success
- `make build` - success
- `make test` - all tests pass

**Result:** Task complete. All 6 org-required validations now use the shared helper with correct exit code 1 behavior.

---

### Task #14: Inconsistent Empty List Handling

**Status:** DONE

**Analysis:**
Currently only `organizations list` prints "No organizations found." when results are empty. All other list commands show an empty table with just headers.

| Command | Current Behavior |
|---------|-----------------|
| `organizations list` | ✓ "No organizations found." |
| `workspaces list` | Empty table |
| `projects list` | Empty table |
| `runs list` | Empty table |
| `workspace-variables list` | Empty table |
| `workspace-resources list` | Empty table |
| `configuration-versions list` | Empty table |
| `invoices list` | Empty table |
| `contexts list` | Empty table |

**Acceptance Criteria:**
- All list commands show "No X found." message when results are empty (table output only)
- JSON output continues to return empty data array (for machine parsing)
- Consistent message format: "No {resource}s found." (lowercase plural)

**Verification:**
- `make test` passes
- Manual verification of empty list behavior in table mode

**Implementation Plan:**
1. Add empty list check with message to `workspaces list`
2. Add empty list check with message to `projects list`
3. Add empty list check with message to `runs list`
4. Add empty list check with message to `workspace-variables list`
5. Add empty list check with message to `workspace-resources list`
6. Add empty list check with message to `configuration-versions list`
7. Add empty list check with message to `invoices list`
8. Add empty list check with message to `contexts list`
9. Add tests for empty list behavior

**Progress Notes (2026-01-23):**

Completed implementation of consistent empty list handling across all list commands.

**Files Changed:**
- `cmd/tfc/workspaces.go` - Added "No workspaces found." message for empty table output
- `cmd/tfc/projects.go` - Added "No projects found." message for empty table output
- `cmd/tfc/runs.go` - Added "No runs found." message for empty table output
- `cmd/tfc/workspace_variables.go` - Added "No variables found." message for empty table output
- `cmd/tfc/workspace_resources.go` - Added "No resources found." message for empty table output
- `cmd/tfc/configuration_versions.go` - Added "No configuration versions found." message for empty table output
- `cmd/tfc/invoices.go` - Added "No invoices found." message for empty table output
- `cmd/tfc/contexts.go` - Added "No contexts found." message for empty table output
- `cmd/tfc/configuration_versions_test.go` - Updated test to expect new message
- `cmd/tfc/runs_test.go` - Updated test to expect new message

**Commands Run:**
- `make fmt` - success
- `make lint` - success
- `make build` - success
- `make test` - all tests pass

**Result:** Task complete. All 9 list commands now show consistent "No X found." messages for empty results in table output mode. JSON output continues to return empty data arrays for machine parsing.
