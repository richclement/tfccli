# Workspaces Subcommand Code Review

Review of `cmd/tfc/workspaces.go` and `cmd/tfc/workspaces_test.go`.

---

## Edge Cases

### 1. Description Cannot Be Cleared

**File:** `cmd/tfc/workspaces.go:341-343` (create) and `417-419` (update)

**Problem:** The empty string check `if c.Description != ""` means users cannot clear a workspace description by passing `--description ""`. Once set, a description cannot be removed.

**Current code:**
```go
if c.Description != "" {
    opts.Description = tfe.String(c.Description)
}
```

**Impact:** Users have no way to remove a description once it's set.

**Possible fix options:**
1. Use a pointer type (`*string`) for the Description field and check for nil vs empty
2. Add a separate `--clear-description` boolean flag
3. Accept a sentinel value like `"-"` to mean "clear"

**Note:** This same pattern exists in other commands. Decide on a consistent approach across the codebase before fixing.

---

## Code Quality Improvements

### 2. Extract Duplicate `resolveClientConfig` Function

**Status:** DONE

**Files:** `cmd/tfc/workspaces.go:107-145` and `cmd/tfc/projects.go:112-150`

**Problem:** `resolveWorkspacesClientConfig` and `resolveProjectsClientConfig` are identical functions. This violates DRY and increases maintenance burden.

#### Plan (2026-01-21)

**Acceptance criteria:**
- Shared `resolveClientConfig` helper exists in `cmd/tfc/common.go`
- `resolveWorkspacesClientConfig` and `resolveProjectsClientConfig` are replaced with calls to shared helper
- `resolveVariablesClientConfig` and `resolveWorkspaceResourcesClientConfig` are also updated (they have similar duplication)
- All existing tests pass
- No behavior changes

**Verification approach:**
- `make test` passes
- All command tests still pass
- Code compiles without errors

**Implementation steps:**
1. Create `cmd/tfc/common.go` with shared `resolveClientConfig` function
2. Update `cmd/tfc/workspaces.go` to use shared helper (remove local function)
3. Update `cmd/tfc/projects.go` to use shared helper (remove local function)
4. Update `cmd/tfc/workspace_variables.go` to use shared helper
5. Update `cmd/tfc/workspace_resources.go` to use shared helper
6. Run feedback loops

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/common.go` (new): Created shared helper file with `resolveClientConfig` and `resolveFormat` functions
- `cmd/tfc/workspaces.go`: Removed `resolveWorkspacesClientConfig`, updated to use shared `resolveClientConfig`
- `cmd/tfc/projects.go`: Removed `resolveProjectsClientConfig` and `resolveFormat`, updated to use shared versions
- `cmd/tfc/workspace_variables.go`: Removed `resolveVariablesClientConfig`, updated to use shared `resolveClientConfig` (ignoring org return)
- `cmd/tfc/workspace_resources.go`: Removed `resolveWorkspaceResourcesClientConfig`, updated to use shared `resolveClientConfig` (ignoring org return)

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)

**What remains:**
- Task #2 is DONE
- Net reduction of ~140 lines of duplicate code
- Also resolves task #7 (resolveVariablesClientConfig), #29 (resolveWorkspaceResourcesClientConfig), and task #4 (resolveFormat helper moved to common.go)

**Fix:** Create a shared helper in a common location (e.g., `cmd/tfc/common.go` or reuse an existing shared file):

```go
// resolveClientConfig resolves settings and token for API calls, including org resolution.
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
```

Then update both files to use this shared function.

---

### 3. Extract Duplicate Test Helper Types

**File:** `cmd/tfc/workspaces_test.go:94-117`

**Problem:** `workspacesTestEnv` and `workspacesTestFS` duplicate the same types defined in other test files. Recent commit `861f345` extracted prompters to `testhelpers_test.go`.

**Fix:** Move these types to `cmd/tfc/testhelpers_test.go`:

```go
// testEnv implements auth.EnvGetter for testing.
type testEnv struct {
    vars map[string]string
}

func (e *testEnv) Getenv(key string) string {
    return e.vars[key]
}

// testFS implements auth.FSReader for testing.
type testFS struct {
    files   map[string][]byte
    homeDir string
}

func (f *testFS) ReadFile(path string) ([]byte, error) {
    if data, ok := f.files[path]; ok {
        return data, nil
    }
    return nil, os.ErrNotExist
}

func (f *testFS) UserHomeDir() (string, error) {
    return f.homeDir, nil
}
```

Then update `workspaces_test.go` and other test files to use the shared types.

---

### 4. Reuse `resolveFormat` Helper

**Status:** DONE (fixed as part of #2)

**File:** `cmd/tfc/workspaces.go`

**Problem:** The workspaces commands have inline TTY detection (e.g., lines 195-199), while projects.go has a cleaner `resolveFormat` helper (lines 101-109).

**Current inline code in workspaces:**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Either:
1. Move `resolveFormat` from projects.go to a shared location and reuse it
2. Or inline the same pattern consistently

The `resolveFormat` helper is slightly cleaner as it encapsulates the logic and returns both values needed.

---

# Workspace-Variables Subcommand Code Review

Review of `cmd/tfc/workspace_variables.go` and `cmd/tfc/workspace_variables_test.go`.

---

## Edge Cases

### 5. Value Cannot Be Cleared

**File:** `cmd/tfc/workspace_variables.go:349-351`

**Problem:** The empty string check `if c.Value != ""` means users cannot clear a variable value by passing `--value ""`. This is more problematic for variables than descriptions since empty values may be legitimate (e.g., disabling a feature flag).

**Current code:**
```go
if c.Value != "" {
    opts.Value = tfe.String(c.Value)
}
```

**Impact:** Users cannot set a variable's value to an empty string.

**Possible fix options:**
1. Use a pointer type (`*string`) for the Value field and check for nil vs empty
2. Add a separate `--clear-value` boolean flag
3. Accept a sentinel value like `"-"` to mean "clear"

**Note:** This same pattern exists for Description (line 352-354). Decide on a consistent approach across the codebase before fixing.

---

### 6. Description Cannot Be Cleared

**File:** `cmd/tfc/workspace_variables.go:352-354`

**Problem:** Same issue as #5 and the workspaces command - users cannot clear a variable description by passing `--description ""`.

**Current code:**
```go
if c.Description != "" {
    opts.Description = tfe.String(c.Description)
}
```

**Fix:** Same approach as chosen for #5.

---

## Code Quality Improvements

### 7. Duplicate `resolveVariablesClientConfig` Function

**Status:** DONE (fixed as part of #2)

**File:** `cmd/tfc/workspace_variables.go:112-144`

**Problem:** `resolveVariablesClientConfig` is nearly identical to `resolveWorkspacesClientConfig` and `resolveProjectsClientConfig`. The only difference is that variables don't return an org (since workspace ID is passed directly).

**Current code:** 33 lines of duplicate config resolution logic.

**Fix:** Create a shared helper in `cmd/tfc/common.go`. The workspace-variables version can call the shared helper and discard the org return value:

```go
// In workspace_variables.go:
cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
```

Or create two variants:
- `resolveClientConfig` - returns (config, org, error) for commands needing org
- `resolveClientConfigNoOrg` - returns (config, error) for commands that don't need org

---

### 8. Duplicate Test Helper Types

**File:** `cmd/tfc/workspace_variables_test.go:72-126`

**Problem:** The test file defines its own prompter and env/fs types that duplicate existing shared types:
- `varsTestEnv` duplicates functionality available in other test files
- `varsTestFS` duplicates functionality available in other test files
- `varsAcceptingPrompter` duplicates `acceptingPrompter` in `testhelpers_test.go`
- `varsRejectingPrompter` duplicates `rejectingPrompter` in `testhelpers_test.go`

**Fix:**
1. Delete `varsAcceptingPrompter` and `varsRejectingPrompter`, use shared types from `testhelpers_test.go`
2. Move `varsTestEnv` and `varsTestFS` to `testhelpers_test.go` as `testEnv` and `testFS`, then update all test files to use the shared types

**Lines to remove:** 72-126 (after moving env/fs types to shared location)

**Updates needed:**
```go
// Replace:
prompter := &varsRejectingPrompter{}
// With:
prompter := &rejectingPrompter{}

// Replace:
prompter := &varsAcceptingPrompter{}
// With:
prompter := &acceptingPrompter{}
```

---

### 9. Inline TTY Detection Pattern

**File:** `cmd/tfc/workspace_variables.go:191-195, 286-290, 372-376, 459-463`

**Problem:** Same inline TTY detection pattern repeated 4 times. This could use the `resolveFormat` helper pattern from `projects.go`.

**Current inline code (repeated 4 times):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Use a shared helper function as suggested in finding #4. After creating the shared helper, update all 4 locations in workspace_variables.go.

---

# Init Subcommand Code Review

Review of `cmd/tfc/main.go` (InitCmd) and `cmd/tfc/init_test.go`.

---

## Edge Cases

### 10. Console Output Not Testable

**Status:** DONE

**File:** `cmd/tfc/main.go:290, 355`

**Problem:** Unlike other commands (e.g., `DoctorCmd`, `WorkspacesListCmd`) that use an injectable `stdout` writer, `InitCmd` writes directly to `os.Stdout` via `fmt.Println()` and `fmt.Printf()`. This makes output untestable and inconsistent with the rest of the codebase.

#### Plan (2026-01-21)

**Acceptance criteria (from PRD Section 14):**
- InitCmd has an injectable `stdout io.Writer` field like other commands
- Default to `os.Stdout` when not injected
- Existing tests still pass
- New tests can verify output content

**Verification approach:**
- `make test` passes
- Existing init tests still pass
- Add test that injects stdout and verifies message content

**Implementation steps:**
1. Add `stdout io.Writer` field to `InitCmd` struct
2. Default to `os.Stdout` in `Run()` if nil
3. Replace `fmt.Println` with `fmt.Fprintln(c.stdout, ...)`
4. Replace `fmt.Printf` with `fmt.Fprintf(c.stdout, ...)`
5. Add test verifying abort message output
6. Add test verifying success message output

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/main.go`:
  - Added `stdout io.Writer` field to `InitCmd` struct (line 267)
  - Added default initialization `if c.stdout == nil { c.stdout = os.Stdout }` in `Run()` (lines 276-278)
  - Replaced `fmt.Println` with `fmt.Fprintln(c.stdout, ...)` (line 302)
  - Replaced `fmt.Printf` with `fmt.Fprintf(c.stdout, ...)` (line 372)
- `cmd/tfc/init_test.go`:
  - Added `TestInitCmd_OutputAbortMessage` - verifies abort message when user declines overwrite
  - Added `TestInitCmd_OutputSuccessMessage` - verifies success message with settings path

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all 17 init tests green, including 2 new tests)

**What remains:**
- Task #10 is complete
- Tests can now inject `stdout` to verify output content

---

## Missing Unit Tests

### 11. Missing Test: SettingsPath Error

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `config.SettingsPath()` fails. This is difficult to trigger since `SettingsPath` only fails if `os.UserHomeDir()` fails and no `baseDir` is provided. Consider adding a test with a mock or skip if not feasible.

**Note:** This is a low-priority test since the error path is unlikely in practice (only fails if HOME env var is unset and no baseDir override). The error is properly wrapped with RuntimeError at line 273-274.

---

# Doctor Subcommand Code Review

Review of `cmd/tfc/main.go` (DoctorCmd, lines 64-248) and `cmd/tfc/doctor_test.go`.

---

## Edge Cases

### 12. Context Not Found Error Message Lacks Guidance

**Status:** DONE

**File:** `cmd/tfc/main.go:135-143`

**Problem:** When a context is not found (either via `--context` flag or misconfigured `current_context`), the error message only says `context "name" not found`. Unlike the settings error which suggests `run 'tfc init'`, this error doesn't guide users toward resolution.

**Current code:**
```go
if !exists {
    result.Checks = append(result.Checks, DoctorCheck{
        Name:   "context",
        Status: string(output.StatusFail),
        Detail: fmt.Sprintf("context %q not found", contextName),
    })
    hasFailure = true
    return d.outputAndError(result, format, isTTY, hasFailure)
}
```

**Fix:** Add guidance to the error message:
```go
Detail: fmt.Sprintf("context %q not found; run 'tfc contexts list' to see available contexts", contextName),
```

#### Plan (2026-01-21)

**Acceptance criteria (from PRD Section 8):**
- Error message for context not found includes actionable guidance
- Guidance points user to `tfc contexts list` to discover available contexts
- Consistent with existing pattern (settings error suggests `tfc init`)

**Verification approach:**
- `make test` passes
- Existing doctor tests still pass
- Test verifies error message contains guidance text

**Implementation steps:**
1. Update error message in `cmd/tfc/main.go:145` to include guidance
2. Update existing test `TestDoctorCmd_ContextNotFound` to verify guidance text
3. Run feedback loops

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/main.go:145`: Updated error message from `context %q not found` to `context %q not found; run 'tfc contexts list' to see available contexts`
- `cmd/tfc/doctor_test.go:712-714`: Added assertion in `TestDoctor_ContextNotFound` to verify guidance text

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)

**What remains:**
- Task #12 is complete
- Error now provides actionable guidance consistent with settings error pattern

---

## Code Quality Improvements

### 13. Duplicate Test Helper Types

**File:** `cmd/tfc/doctor_test.go:19-43`

**Problem:** `fakeEnv` and `fakeFS` types in doctor_test.go duplicate similar types in other test files. Recent commit `861f345` extracted prompters to `testhelpers_test.go`, but env/fs helpers were not consolidated.

**Current code (doctor_test.go):**
```go
// fakeEnv implements auth.EnvGetter for testing.
type fakeEnv struct {
    vars map[string]string
}

func (e *fakeEnv) Getenv(key string) string {
    return e.vars[key]
}

// fakeFS implements auth.FSReader for testing.
type fakeFS struct {
    files   map[string][]byte
    homeDir string
}

func (f *fakeFS) ReadFile(path string) ([]byte, error) {
    if data, ok := f.files[path]; ok {
        return data, nil
    }
    return nil, os.ErrNotExist
}

func (f *fakeFS) UserHomeDir() (string, error) {
    return f.homeDir, nil
}
```

**Fix:** Move these types to `cmd/tfc/testhelpers_test.go` and update all test files to use the shared types. This aligns with finding #3 and #8 which identified the same duplication pattern in other test files.

---

### 14. Consider Using `resolveFormat` Helper

**File:** `cmd/tfc/main.go:106-107`

**Problem:** The doctor command has inline TTY detection and format resolution, while `projects.go:103` has a `resolveFormat` helper that encapsulates this logic. For consistency, the doctor command could use this helper.

**Current code:**
```go
isTTY := d.ttyDetector.IsTTY(d.stdout)
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Alternative (after moving resolveFormat to shared location):**
```go
format, isTTY := resolveFormat(d.stdout, d.ttyDetector, cli.OutputFormat)
```

**Note:** This depends on first moving `resolveFormat` from `projects.go` to a shared location as suggested in finding #4.

---

# Contexts Subcommand Code Review

Review of `cmd/tfc/main.go` (ContextsCmd, lines 359-540) and `cmd/tfc/contexts_test.go`.

---

## Bugs

### 15. No JSON Output Format Support (DONE)

**Status:** DONE

**File:** `cmd/tfc/main.go:381-460` (list), `584-658` (show)

**Problem:** The contexts commands use `fmt.Printf` directly without supporting `--output-format json/table` like other commands. This is inconsistent with the rest of the codebase (projects, workspaces, doctor, etc.) which all support both JSON and table output.

#### Plan (2026-01-21)

**Acceptance criteria (from PRD):**
- `--output-format table|json` flag works for `contexts list` and `contexts show`
- Default to JSON when stdout is not a TTY, table when it is
- JSON output for list: array of objects with `name`, `is_current` fields
- JSON output for show: object with `name`, `is_current`, `address`, `default_org`, `log_level` fields

**Implementation steps:**
1. Add `stdout`, `ttyDetector` fields to `ContextsListCmd` and `ContextsShowCmd`
2. Update `ContextsListCmd.Run()` to accept `cli *CLI` parameter
3. Update `ContextsShowCmd.Run()` to accept `cli *CLI` parameter
4. Implement JSON and table output for list command
5. Implement JSON and table output for show command
6. Add tests for JSON output format

**Verification:**
- `make test` passes
- Existing tests still pass
- New tests cover JSON output

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/main.go`: Added JSON/table output support to `ContextsListCmd` and `ContextsShowCmd`
  - Added `stdout`, `ttyDetector` fields for testability
  - Added `contextListItem` and `contextShowItem` structs for JSON serialization
  - Updated `Run()` methods to accept `cli *CLI` parameter
  - List outputs array of items with `name`, `is_current`
  - Show outputs single item with `name`, `is_current`, `address`, `default_org`, `log_level`
  - Table output uses `output.NewTableWriter` for consistency
  - Also fixes #17: Empty default_org now displays "(none)" in table output
- `cmd/tfc/contexts_test.go`: Updated existing tests and added new JSON output tests
  - Updated all `Run()` calls to pass `cli *CLI` parameter
  - Added `TestContextsListCmd_JSONOutput`
  - Added `TestContextsShowCmd_JSONOutput`
  - Added `TestContextsShowCmd_EmptyDefaultOrgDisplayed`
  - Enhanced existing tests to verify output content

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)

**What remains:**
- Task #15 is complete
- Task #17 (Empty default org display) was also fixed as part of this change

**Current code (list):**
```go
func (c *ContextsListCmd) Run() error {
    settings, err := config.Load(c.baseDir)
    // ...
    for name := range settings.Contexts {
        marker := "  "
        if name == settings.CurrentContext {
            marker = "* "
        }
        fmt.Printf("%s%s\n", marker, name)
    }
    return nil
}
```

**Fix:**

1. Add `stdout`, `ttyDetector` fields to `ContextsListCmd`:
```go
type ContextsListCmd struct {
    baseDir     string
    stdout      io.Writer
    ttyDetector output.TTYDetector
}
```

2. Add JSON output struct:
```go
type contextListItem struct {
    Name      string `json:"name"`
    IsCurrent bool   `json:"is_current"`
}
```

3. Update Run method to support both formats:
```go
func (c *ContextsListCmd) Run(cli *CLI) error {
    if c.stdout == nil {
        c.stdout = os.Stdout
    }
    if c.ttyDetector == nil {
        c.ttyDetector = &output.RealTTYDetector{}
    }

    settings, err := config.Load(c.baseDir)
    if err != nil {
        return internalcmd.NewRuntimeError(err)
    }

    format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

    if format == output.FormatJSON {
        items := make([]contextListItem, 0, len(settings.Contexts))
        for name := range settings.Contexts {
            items = append(items, contextListItem{
                Name:      name,
                IsCurrent: name == settings.CurrentContext,
            })
        }
        // Sort for consistent output
        sort.Slice(items, func(i, j int) bool {
            return items[i].Name < items[j].Name
        })
        return output.WriteJSON(c.stdout, items)
    }

    // Table output
    tw := output.NewTableWriter(c.stdout, []string{"", "NAME"}, isTTY)
    // ... collect and sort names, then render
}
```

**Note:** Same fix needed for `ContextsShowCmd`.

---

### 16. Console Output Not Testable

**Status:** DONE

**File:** `cmd/tfc/main.go`

**Problem:** Some contexts commands write directly to `os.Stdout` via `fmt.Printf()` and `fmt.Println()`, making output untestable. Other commands (ProjectsListCmd, WorkspacesListCmd, DoctorCmd) have injectable `stdout` fields.

#### Plan (2026-01-21)

**Acceptance criteria:**
- `ContextsAddCmd`, `ContextsUseCmd`, and `ContextsRemoveCmd` have injectable `stdout io.Writer` fields
- All `fmt.Printf`/`fmt.Println` replaced with `fmt.Fprintf(c.stdout, ...)`
- Default to `os.Stdout` when not injected
- Existing tests still pass
- New tests can verify output content

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/main.go`:
  - Added `stdout io.Writer` field to `ContextsAddCmd`
  - Added `stdout io.Writer` field to `ContextsUseCmd`
  - Added `stdout io.Writer` field to `ContextsRemoveCmd`
  - Each `Run()` method defaults `stdout` to `os.Stdout` if nil
  - Replaced all `fmt.Printf`/`fmt.Println` with `fmt.Fprintf`/`fmt.Fprintln` using `c.stdout`

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)

**What remains:**
- Task #16 is complete
- Tests can now use injectable `stdout` to verify output content

---

## Edge Cases

### 17. Show Command Displays Empty Default Org (DONE)

**Status:** DONE (fixed as part of #15)

**File:** `cmd/tfc/main.go:537`

**Problem:** When `--default-org` is not set, `ContextsShowCmd` displays `Default Org:` followed by nothing, which looks incomplete and could confuse users.

**Fix applied:** Empty default_org now displays "(none)" in table output. Test added: `TestContextsShowCmd_EmptyDefaultOrgDisplayed`.

---

### 18. ContextsListCmd Signature Missing CLI Parameter

**File:** `cmd/tfc/main.go:373`

**Problem:** `ContextsListCmd.Run()` takes no parameters, unlike other list commands which take `cli *CLI`. This prevents access to the `--output-format` flag, which is why JSON output isn't supported. The same applies to `ContextsShowCmd.Run()`.

**Current code:**
```go
func (c *ContextsListCmd) Run() error {
```

**Fix:** Update signature to accept CLI:
```go
func (c *ContextsListCmd) Run(cli *CLI) error {
```

**Note:** Kong will automatically inject the CLI parameter via the `kong.Bind(&cli)` call in `run()`.

---

### 19. ContextsAddCmd Needs CLI Parameter for Consistency

**File:** `cmd/tfc/main.go:399`

**Problem:** `ContextsAddCmd.Run()` and `ContextsUseCmd.Run()` don't take a `cli *CLI` parameter, which is inconsistent with other commands and prevents future enhancements like JSON output confirmation messages.

**Current code:**
```go
func (c *ContextsAddCmd) Run() error {
func (c *ContextsUseCmd) Run() error {
```

**Fix:** Add CLI parameter for consistency:
```go
func (c *ContextsAddCmd) Run(cli *CLI) error {
func (c *ContextsUseCmd) Run(cli *CLI) error {
```

---

## Missing Unit Tests

### 20. Missing Test: ContextsRemoveCmd config.Save Failure

**Status:** DONE

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Save()` fails for the remove command.

#### Plan (2026-01-21)

**Acceptance criteria:**
- Test exists that triggers `config.Save()` failure in `ContextsRemoveCmd.Run()`
- Test verifies error message contains "failed to save settings"
- Test uses same pattern as `TestContextsUseCmd_SaveError` for consistency

**Verification approach:**
- `make test` passes
- New test specifically tests the save failure code path (main.go:585-587)

**Implementation steps:**
1. Add `TestContextsRemoveCmd_SaveError` test following pattern from `TestContextsUseCmd_SaveError`
2. Create settings with two contexts (default and prod)
3. Make settings file read-only to cause save failure
4. Attempt to remove non-current context with --force
5. Verify error contains "failed to save settings"
6. Run feedback loops

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/contexts_test.go`: Added `TestContextsRemoveCmd_SaveError` test (lines 682-714)
  - Creates test settings with "default" and "prod" contexts
  - Makes settings file read-only (0o400) to trigger save failure
  - Uses `forceFlag: &forceVal` to bypass confirmation prompt
  - Verifies error contains "failed to save settings"

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)
- `go test -v -run TestContextsRemoveCmd_SaveError ./cmd/tfc/...` - passed

**What remains:**
- Task #20 is complete
- Test coverage now includes save failure path for ContextsRemoveCmd

**Test to add:**
```go
// TestContextsRemoveCmd_SaveError tests that save errors are properly surfaced.
func TestContextsRemoveCmd_SaveError(t *testing.T) {
    tmpHome := t.TempDir()

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
            "prod":    {Address: "tfe.example.com", LogLevel: "warn"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    // Make directory read-only
    tfccliDir := filepath.Join(tmpHome, ".tfccli")
    if err := os.Chmod(tfccliDir, 0o500); err != nil {
        t.Fatalf("Failed to chmod: %v", err)
    }
    t.Cleanup(func() {
        os.Chmod(tfccliDir, 0o700)
    })

    forceVal := true
    cmd := &ContextsRemoveCmd{
        Name:      "prod",
        baseDir:   tmpHome,
        forceFlag: &forceVal,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when save fails, got nil")
    }
    if !strings.Contains(err.Error(), "failed to save settings") {
        t.Errorf("expected save failure message, got: %v", err)
    }
}
```

---

### 21. Tests Don't Verify Output Content

**File:** `cmd/tfc/contexts_test.go:18-38, 306-326`

**Problem:** Several tests pass but include comments noting they don't verify stdout content. After fixing #16 (injectable stdout), these tests should be updated to capture and verify output.

**Tests that need output verification:**
- `TestContextsListCmd_ListsAllContexts` (line 18)
- `TestContextsShowCmd_ShowsCurrentContext` (line 306)
- `TestContextsShowCmd_ShowsNamedContext` (line 328)

**Example fix for `TestContextsListCmd_ListsAllContexts`:**
```go
func TestContextsListCmd_ListsAllContexts(t *testing.T) {
    tmpHome := t.TempDir()
    out := &bytes.Buffer{}

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
            "prod":    {Address: "tfe.example.com", LogLevel: "warn"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    cmd := &ContextsListCmd{
        baseDir: tmpHome,
        stdout:  out,
    }

    err := cmd.Run()
    if err != nil {
        t.Fatalf("Run() error = %v", err)
    }

    output := out.String()
    if !strings.Contains(output, "* default") {
        t.Errorf("expected current context marked with *, got: %s", output)
    }
    if !strings.Contains(output, "prod") {
        t.Errorf("expected 'prod' in output, got: %s", output)
    }
}
```

---

### 22. Missing Test: ContextsListCmd JSON Output

**File:** `cmd/tfc/contexts_test.go`

**Problem:** After implementing JSON output support (#15), needs tests for JSON format.

**Test to add (after #15 is fixed):**
```go
// TestContextsListCmd_JSONOutput tests JSON output format.
func TestContextsListCmd_JSONOutput(t *testing.T) {
    tmpHome := t.TempDir()
    out := &bytes.Buffer{}

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
            "prod":    {Address: "tfe.example.com", LogLevel: "warn"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    cmd := &ContextsListCmd{
        baseDir:     tmpHome,
        stdout:      out,
        ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
    }
    cli := &CLI{OutputFormat: "json"}

    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("Run() error = %v", err)
    }

    var items []map[string]interface{}
    if err := json.Unmarshal(out.Bytes(), &items); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    if len(items) != 2 {
        t.Errorf("expected 2 items, got %d", len(items))
    }
}
```

---

### 23. Missing Test: ContextsShowCmd JSON Output

**File:** `cmd/tfc/contexts_test.go`

**Problem:** After implementing JSON output support, needs test for JSON format in show command.

**Test to add (after #15 is fixed):**
```go
// TestContextsShowCmd_JSONOutput tests JSON output format.
func TestContextsShowCmd_JSONOutput(t *testing.T) {
    tmpHome := t.TempDir()
    out := &bytes.Buffer{}

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", DefaultOrg: "acme", LogLevel: "info"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    cmd := &ContextsShowCmd{
        baseDir:     tmpHome,
        stdout:      out,
        ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
    }
    cli := &CLI{OutputFormat: "json"}

    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("Run() error = %v", err)
    }

    var result map[string]interface{}
    if err := json.Unmarshal(out.Bytes(), &result); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    if result["name"] != "default" {
        t.Errorf("expected name 'default', got: %v", result["name"])
    }
}
```

---

# Workspace-Resources Subcommand Code Review

Review of `cmd/tfc/workspace_resources.go` and `cmd/tfc/workspace_resources_test.go`.

---

## Missing Features

### 24. Missing Get Subcommand

**File:** `cmd/tfc/workspace_resources.go:18-21`

**Problem:** The `WorkspaceResourcesCmd` only implements a `List` subcommand. Unlike other resource commands (`ProjectsCmd`, `WorkspacesCmd`, `WorkspaceVariablesCmd`) which have Get/Read operations, there's no way to retrieve a single workspace resource by ID.

**Current code:**
```go
type WorkspaceResourcesCmd struct {
    List WorkspaceResourcesListCmd `cmd:"" help:"List resources in a workspace."`
}
```

**Impact:** Users must list all resources and filter client-side to find a specific resource's details.

**Fix:** Add a `WorkspaceResourcesGetCmd`:

```go
type WorkspaceResourcesCmd struct {
    List WorkspaceResourcesListCmd `cmd:"" help:"List resources in a workspace."`
    Get  WorkspaceResourcesGetCmd  `cmd:"" help:"Get a resource by ID."`
}

// WorkspaceResourcesGetCmd retrieves a single workspace resource.
type WorkspaceResourcesGetCmd struct {
    ResourceID  string `arg:"" help:"ID of the resource to retrieve."`
    WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

    // Dependencies for testing
    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        io.Writer
    clientFactory workspaceResourcesClientFactory
}
```

**Note:** Also need to add `Read` method to the `workspaceResourcesClient` interface:
```go
type workspaceResourcesClient interface {
    List(ctx context.Context, workspaceID string, opts *tfe.WorkspaceResourceListOptions) ([]*tfe.WorkspaceResource, error)
    Read(ctx context.Context, workspaceID string, resourceID string) (*tfe.WorkspaceResource, error)
}
```

**Tests to add:** `TestWorkspaceResourcesGet_JSON`, `TestWorkspaceResourcesGet_Table`, `TestWorkspaceResourcesGet_NotFound`, `TestWorkspaceResourcesGet_APIError`.

---

## Missing Unit Tests

### 25. Missing Test: Client Factory Error

**Status:** DONE

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error. This tests lines 148-151 in the Run method.

#### Plan (2026-01-21)

**Acceptance criteria:**
- Test exists that triggers `clientFactory` failure in `WorkspaceResourcesListCmd.Run()`
- Test verifies error message contains "failed to create client"
- Test uses same pattern as other error tests in the file (e.g., `TestWorkspaceResourcesList_APIError`)

**Verification approach:**
- `make test` passes
- New test specifically tests lines 148-151 in workspace_resources.go

**Implementation steps:**
1. Add `TestWorkspaceResourcesList_ClientFactoryError` test following pattern from review.md
2. Run feedback loops
3. Update review.md with results

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/workspace_resources_test.go`: Added `TestWorkspaceResourcesList_ClientFactoryError` test (lines 400-421)
  - Reuses `setupWorkspaceResourcesTestSettings` helper for consistency
  - Injects a `clientFactory` that returns an error
  - Verifies error contains "failed to create client"

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)
- `go test -v -run TestWorkspaceResourcesList_ClientFactoryError ./cmd/tfc/...` - passed

**What remains:**
- Task #25 is complete
- Test coverage now includes client factory failure path (workspace_resources.go:148-151)

**Test to add:**
```go
// TestWorkspaceResourcesList_ClientFactoryError tests error when client factory fails.
func TestWorkspaceResourcesList_ClientFactoryError(t *testing.T) {
    tmpDir, resolver := setupWorkspaceResourcesTestSettings(t)

    var buf bytes.Buffer
    cmd := &WorkspaceResourcesListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
            return nil, errors.New("failed to initialize TFC client")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected 'failed to create client' in error, got: %v", err)
    }
}
```

---

### 26. Missing Test: Context Not Found

**Status:** DONE

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when the specified `--context` flag references a non-existent context.

#### Plan (2026-01-21)

**Acceptance criteria (from PRD Section 6):**
- Test exists that triggers "context not found" error in `WorkspaceResourcesListCmd.Run()`
- Test verifies error message contains "context" and "not found"
- Test uses same pattern as other error tests in the file (e.g., `TestWorkspaceResourcesList_FailsWhenSettingsMissing`)

**Verification approach:**
- `make test` passes
- New test specifically tests line 96-98 in workspace_resources.go (context lookup failure)

**Implementation steps:**
1. Add `TestWorkspaceResourcesList_ContextNotFound` test following pattern from review.md
2. Create settings with only "default" context
3. Use `cli.Context = "nonexistent"` to trigger context not found
4. Verify error contains "context" and "not found"
5. Run feedback loops
6. Update review.md with results

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/workspace_resources_test.go`: Added `TestWorkspaceResourcesList_ContextNotFound` test (lines 426-474)
  - Creates test settings with only "default" context
  - Uses `cli.Context = "nonexistent"` to trigger context lookup failure
  - Verifies error contains "context" and "not found"
  - Follows same pattern as other error tests in file

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)
- `go test -v -run TestWorkspaceResourcesList_ContextNotFound ./cmd/tfc/...` - passed

**What remains:**
- Task #26 is complete
- Test coverage now includes context not found error path (workspace_resources.go:96-98)

**Test to add:**
```go
// TestWorkspaceResourcesList_ContextNotFound tests error when context doesn't exist.
func TestWorkspaceResourcesList_ContextNotFound(t *testing.T) {
    tmpDir := t.TempDir()

    // Create settings with only "default" context
    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {
                Address:  "app.terraform.io",
                LogLevel: "info",
            },
        },
    }
    if err := config.Save(settings, tmpDir); err != nil {
        t.Fatalf("failed to save test settings: %v", err)
    }

    fakeEnv := &wsrTestEnv{
        vars: map[string]string{
            "TF_TOKEN_app_terraform_io": "test-token",
        },
    }
    fakeFS := &wsrTestFS{
        homeDir: tmpDir,
        files:   make(map[string][]byte),
    }
    resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

    var buf bytes.Buffer
    cmd := &WorkspaceResourcesListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
            return &fakeWorkspaceResourcesClient{}, nil
        },
    }

    // Use --context flag to select nonexistent context
    cli := &CLI{OutputFormat: "json", Context: "nonexistent"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when context not found, got nil")
    }
    if !strings.Contains(err.Error(), "context") || !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected 'context not found' error, got: %v", err)
    }
}
```

---

### 27. Missing Test: Token Resolution Error

**Status:** DONE

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when token resolution fails (e.g., no token available for the address).

#### Plan (2026-01-21)

**Acceptance criteria (from PRD Section 8):**
- Test exists that triggers token resolution error in `WorkspaceResourcesListCmd.Run()`
- Test verifies error message indicates no token found
- Test uses same pattern as other error tests in the file (e.g., `TestWorkspaceResourcesList_FailsWhenSettingsMissing`)

**Verification approach:**
- `make test` passes
- New test specifically tests line 104-107 in workspace_resources.go (token resolution failure)

**Implementation steps:**
1. Add `TestWorkspaceResourcesList_TokenResolutionError` test following pattern from review.md
2. Create settings with valid context
3. Create resolver with no tokens available (empty env vars and no credentials file)
4. Verify error message indicates token discovery failure
5. Run feedback loops
6. Update review.md with results

**Test to add:**
```go
// TestWorkspaceResourcesList_TokenResolutionError tests error when no token is available.
func TestWorkspaceResourcesList_TokenResolutionError(t *testing.T) {
    tmpDir := t.TempDir()

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {
                Address:  "app.terraform.io",
                LogLevel: "info",
            },
        },
    }
    if err := config.Save(settings, tmpDir); err != nil {
        t.Fatalf("failed to save test settings: %v", err)
    }

    // Create resolver with no tokens available
    fakeEnv := &wsrTestEnv{
        vars: map[string]string{}, // No tokens
    }
    fakeFS := &wsrTestFS{
        homeDir: tmpDir,
        files:   make(map[string][]byte), // No credentials file
    }
    resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

    var buf bytes.Buffer
    cmd := &WorkspaceResourcesListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
            return &fakeWorkspaceResourcesClient{}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when no token available, got nil")
    }
    if !strings.Contains(err.Error(), "token") {
        t.Errorf("expected token-related error, got: %v", err)
    }
}
```

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/workspace_resources_test.go`: Added `TestWorkspaceResourcesList_TokenResolutionError` test (lines 479-522)
  - Creates test settings with valid "default" context
  - Creates resolver with empty environment variables and no credentials files
  - Verifies error message contains "token" indicating token discovery failure

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)
- `go test -v -run TestWorkspaceResourcesList_TokenResolutionError ./cmd/tfc/...` - passed

**What remains:**
- Task #27 is complete
- Test coverage now includes token resolution failure path (workspace_resources.go:104-107)

---

### 28. Missing Test: Table Output Column Verification

**Status:** DONE

**File:** `cmd/tfc/workspace_resources_test.go:144-180`

**Problem:** `TestWorkspaceResourcesList_Table` only verifies that headers and resource ID/name appear in output, but doesn't verify the actual column order or that the correct data appears in the correct columns.

#### Plan (2026-01-21)

**Acceptance criteria (from PRD Section 6):**
- Test exists that verifies table columns have correct headers in expected order
- Test verifies data appears in correct columns (ID maps to resource ID, RESOURCE-TYPE to ProviderType, NAME to Name, PROVIDER to Provider)
- Test verifies multiple rows render correctly

**Verification approach:**
- `make test` passes
- New test specifically validates column structure and data alignment

**Implementation steps:**
1. Add `TestWorkspaceResourcesList_Table_ColumnVerification` test
2. Create resources with distinct, non-overlapping values per column (to verify correct column placement)
3. Verify header row contains columns in correct order
4. Verify data rows contain expected values
5. Run feedback loops
6. Update review.md with results

#### Progress Note (2026-01-21)

**Files changed:**
- `cmd/tfc/workspace_resources_test.go`: Added `TestWorkspaceResourcesList_Table_ColumnVerification` test (lines 527-609)
  - Creates two resources with distinct values per column to verify correct placement
  - Verifies header contains columns in order: ID, RESOURCE-TYPE, NAME, PROVIDER
  - Verifies separator line exists between header and data
  - Verifies first data row contains: res-abc, aws_instance, webserver, hashicorp/aws
  - Verifies second data row contains: res-xyz, google_compute_instance, database, hashicorp/google

**Commands run:**
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - passed (all tests green)
- `go test -v -run TestWorkspaceResourcesList_Table_ColumnVerification ./cmd/tfc/...` - passed

**What remains:**
- Task #28 is complete
- Test coverage now includes table column structure and data alignment verification

---

## Code Quality Improvements

### 29. Duplicate `resolveWorkspaceResourcesClientConfig` Function

**Status:** DONE (fixed as part of #2)

**File:** `cmd/tfc/workspace_resources.go:84-117`

**Problem:** `resolveWorkspaceResourcesClientConfig` is nearly identical to `resolveVariablesClientConfig` in `workspace_variables.go` and similar functions in other command files. The only difference is that workspace-resources and workspace-variables don't return an org (since workspace ID is passed directly).

**Current code:** 33 lines of duplicate config resolution logic.

**Fix:** Use the shared helper pattern suggested in finding #7. After creating a shared `resolveClientConfig` helper, update workspace_resources.go:

```go
// In workspace_resources.go:
cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
if err != nil {
    return internalcmd.NewRuntimeError(err)
}
```

Or if a `resolveClientConfigNoOrg` variant is created:
```go
cfg, err := resolveClientConfigNoOrg(cli, c.baseDir, c.tokenResolver)
```

---

### 30. Duplicate Test Helper Types

**File:** `cmd/tfc/workspace_resources_test.go:33-57`

**Problem:** `wsrTestEnv` and `wsrTestFS` duplicate the same types defined in other test files (`workspaces_test.go`, `workspace_variables_test.go`, `doctor_test.go`). Recent commit `861f345` extracted prompters to `testhelpers_test.go`, but env/fs helpers were not consolidated.

**Current code:**
```go
// wsrTestEnv implements auth.EnvGetter for testing.
type wsrTestEnv struct {
    vars map[string]string
}

func (e *wsrTestEnv) Getenv(key string) string {
    return e.vars[key]
}

// wsrTestFS implements auth.FSReader for testing.
type wsrTestFS struct {
    files   map[string][]byte
    homeDir string
}

func (f *wsrTestFS) ReadFile(path string) ([]byte, error) {
    if data, ok := f.files[path]; ok {
        return data, nil
    }
    return nil, os.ErrNotExist
}

func (f *wsrTestFS) UserHomeDir() (string, error) {
    return f.homeDir, nil
}
```

**Fix:** Move these types to `cmd/tfc/testhelpers_test.go` as `testEnv` and `testFS`, then update all test files to use the shared types. This aligns with findings #3, #8, and #13 which identified the same duplication pattern.

---

### 31. Inline TTY Detection Pattern

**File:** `cmd/tfc/workspace_resources.go:163-168`

**Problem:** The code has inline TTY detection and format resolution instead of using the `resolveFormat` helper defined in `projects.go:101-109`.

**Current inline code:**
```go
// Determine output format
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** After moving `resolveFormat` from `projects.go` to a shared location (as suggested in finding #4), use the helper:

```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

# Summary

## Remaining Tasks by Category

### Edge Cases (Cannot clear values)
- #1: Workspaces - Description cannot be cleared
- #5: Workspace-Variables - Value cannot be cleared
- #6: Workspace-Variables - Description cannot be cleared

### Console Output Not Testable
- #10: InitCmd - Console output not testable
- #16: Contexts commands - Console output not testable

### Missing JSON Output Support
- #15: Contexts - No JSON output format support

### Error Message Improvements
- #12: Doctor - Context not found error lacks guidance
- #17: Contexts show - Empty default org display

### Method Signature Consistency
- #18: ContextsListCmd missing CLI parameter
- #19: ContextsAddCmd/ContextsUseCmd missing CLI parameter

### Missing Unit Tests
- #11: Init - SettingsPath error (low priority)
- #20: ContextsRemoveCmd - config.Save failure
- #21: Contexts tests - Output content verification
- #22: ContextsListCmd - JSON output test
- #23: ContextsShowCmd - JSON output test
- #25: WorkspaceResources - Client factory error
- #26: WorkspaceResources - Context not found
- #27: WorkspaceResources - Token resolution error
- #28: WorkspaceResources - Table column verification

### Missing Features
- #24: WorkspaceResources - Missing Get subcommand

### Code Quality (DRY violations)
- #2: Extract duplicate resolveClientConfig
- #3: Extract duplicate test helper types (workspaces)
- #4: Reuse resolveFormat helper (workspaces)
- #7: Duplicate resolveVariablesClientConfig
- #8: Duplicate test helper types (workspace-variables)
- #9: Inline TTY detection pattern (workspace-variables)
- #13: Duplicate test helper types (doctor)
- #14: Consider using resolveFormat helper (doctor)
- #29: Duplicate resolveWorkspaceResourcesClientConfig
- #30: Duplicate test helper types (workspace-resources)
- #31: Inline TTY detection pattern (workspace-resources)
