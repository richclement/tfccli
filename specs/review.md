# Workspaces Subcommand Code Review

Review of `cmd/tfc/workspaces.go` and `cmd/tfc/workspaces_test.go`.

---

## Bugs

### 1. Inconsistent Error Type for Missing Org in List Command

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces.go:176-177`

**Problem:** The list command returns a plain error when org is missing, while create (line 328-329) wraps it in `RuntimeError`. This causes different exit codes (1 vs 2).

**Current code:**
```go
if org == "" {
    return fmt.Errorf("organization is required; use --org flag or set default_org in context")
}
```

**Expected behavior:** Should return exit code 2 (runtime error) like the create command and the projects list command (`projects.go:181`).

**Fix:** Change to:
```go
if org == "" {
    return internalcmd.NewRuntimeError(fmt.Errorf("organization is required; use --org flag or set default_org in context"))
}
```

**Progress notes (2026-01-21):**

Plan:
- Acceptance criteria: Both list and create commands should return `internalcmd.RuntimeError` when org is missing
- Verification: Run tests verifying RuntimeError type is returned
- Implementation: Edit workspaces.go:176 and workspaces.go:329, update tests

Changes made:
- `cmd/tfc/workspaces.go:176` - Wrapped error with `internalcmd.NewRuntimeError`
- `cmd/tfc/workspaces.go:329` - Wrapped error with `internalcmd.NewRuntimeError` (also fixed, was same bug)
- `cmd/tfc/workspaces_test.go` - Added import for `internalcmd`, added RuntimeError type assertions to `TestWorkspacesList_FailsWhenNoOrg` and `TestWorkspacesCreate_FailsWhenNoOrg`

Verification:
- `make fmt` - passed
- `make lint` - passed (only cache warnings)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesList_FailsWhenNoOrg|TestWorkspacesCreate_FailsWhenNoOrg" ./cmd/tfc/...` - both pass

---

## Edge Cases

### 2. Update Command Allows No-Op API Calls

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces.go:413-419`

**Problem:** Unlike `ProjectsUpdateCmd` (see `projects.go:386-388`), the workspaces update command doesn't validate that at least one field is being updated. If a user runs `tfc workspaces update ws-123` with no flags, it sends a no-op API call and prints "Workspace updated" even though nothing changed.

**Current code:**
```go
opts := tfe.WorkspaceUpdateOptions{}
if c.Name != "" {
    opts.Name = tfe.String(c.Name)
}
if c.Description != "" {
    opts.Description = tfe.String(c.Description)
}
```

**Fix:** Add validation before the API call (after line 412):
```go
// Validate at least one field is being updated
if c.Name == "" && c.Description == "" {
    return internalcmd.NewRuntimeError(fmt.Errorf("at least one of --name or --description is required"))
}
```

**Test to add:** `TestWorkspacesUpdate_FailsWhenNoFields` - verify error message when neither --name nor --description provided.

**Progress notes (2026-01-21):**

Plan:
- Acceptance criteria: `tfc workspaces update ws-123` (no flags) returns RuntimeError with message "at least one of --name or --description is required"
- Verification: Add test `TestWorkspacesUpdate_FailsWhenNoFields` that verifies error is returned and is RuntimeError type
- Implementation:
  1. Add validation check after defaults setup in `WorkspacesUpdateCmd.Run()`
  2. Add test case in `workspaces_test.go`
  3. Run feedback loops

Changes made:
- `cmd/tfc/workspaces.go:401-403` - Added validation before API call
- `cmd/tfc/workspaces_test.go:580-622` - Added `TestWorkspacesUpdate_FailsWhenNoFields` test

Verification:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesUpdate_FailsWhenNoFields" ./cmd/tfc/...` - pass

---

### 3. Description Cannot Be Cleared

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

## Missing Unit Tests

### 4. Missing Test: Prompter Error Path for Delete

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces_test.go`

**Problem:** No test verifies error handling when `prompter.Confirm()` returns an error. The projects tests have `TestProjectsDelete_PrompterError` but workspaces doesn't.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspacesDelete_PrompterError` exists and verifies that when prompter.Confirm() returns an error, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for confirmation"
- Verification: Run `go test -v -run "TestWorkspacesDelete_PrompterError" ./cmd/tfc/...`
- Implementation:
  1. Use the existing shared `errorPrompter` from `testhelpers_test.go` (not create new `wsErrorPrompter`)
  2. Add test `TestWorkspacesDelete_PrompterError` to `workspaces_test.go`
  3. Verify test passes with `make test`

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces_test.go:842-870` - Added `TestWorkspacesDelete_PrompterError` test using shared `errorPrompter`

Verification:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesDelete_PrompterError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspacesDelete_PrompterError tests that prompter errors are surfaced.
func TestWorkspacesDelete_PrompterError(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    // Create a prompter that returns an error
    errorPrompter := &wsErrorPrompter{err: errors.New("terminal not available")}

    cmd := &WorkspacesDeleteCmd{
        ID:            "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return &fakeWorkspacesClient{}, nil
        },
        prompter: errorPrompter,
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
        t.Errorf("expected prompt error, got: %v", err)
    }
}
```

You'll also need to add a `wsErrorPrompter` type:
```go
// wsErrorPrompter returns an error for Confirm to test error handling.
type wsErrorPrompter struct {
    err error
}

func (p *wsErrorPrompter) PromptString(_, _ string) (string, error) {
    return "", p.err
}

func (p *wsErrorPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, p.err
}

func (p *wsErrorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", p.err
}
```

---

### 5. Missing Test: Create API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces_test.go`

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspacesCreate_APIError` exists and verifies that when the Create API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to create workspace"
- Verification: Run `go test -v -run "TestWorkspacesCreate_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspacesCreate_APIError` to `workspaces_test.go` after `TestWorkspacesCreate_FailsWhenNoOrg`
  2. Verify test passes with `make test`

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces_test.go:536-571` - Added `TestWorkspacesCreate_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesCreate_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspacesCreate_APIError tests that API errors are surfaced during create.
func TestWorkspacesCreate_APIError(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeWorkspacesClient{
        createErr: errors.New("workspace name already exists"),
    }

    cmd := &WorkspacesCreateCmd{
        Name:          "existing-workspace",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to create workspace") {
        t.Errorf("expected create failure message, got: %v", err)
    }
}
```

---

### 6. Missing Test: Update API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces_test.go`

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspacesUpdate_APIError` exists and verifies that when the Update API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to update workspace"
- Verification: Run `go test -v -run "TestWorkspacesUpdate_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspacesUpdate_APIError` to `workspaces_test.go` after `TestWorkspacesUpdate_FailsWhenNoFields`
  2. Verify test passes with `make test`

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces_test.go:657-691` - Added `TestWorkspacesUpdate_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with alternate cache dirs due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesUpdate_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspacesUpdate_APIError tests that API errors are surfaced during update.
func TestWorkspacesUpdate_APIError(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeWorkspacesClient{
        updateErr: errors.New("workspace not found"),
    }

    cmd := &WorkspacesUpdateCmd{
        ID:            "ws-nonexistent",
        Name:          "new-name",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to update workspace") {
        t.Errorf("expected update failure message, got: %v", err)
    }
}
```

---

### 7. Missing Test: Delete API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces_test.go`

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspacesDelete_APIError` exists and verifies that when the Delete API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to delete workspace"
- Verification: Run `go test -v -run "TestWorkspacesDelete_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspacesDelete_APIError` to `workspaces_test.go` after `TestWorkspacesDelete_PrompterError`
  2. Use `fakeWorkspacesClient.deleteErr` to simulate API failure
  3. Use `--force` to skip confirmation prompt and test the API error path directly
  4. Verify RuntimeError type for exit code 2
  5. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces_test.go:944-981` - Added `TestWorkspacesDelete_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesDelete_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspacesDelete_APIError tests that API errors are surfaced during delete.
func TestWorkspacesDelete_APIError(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeWorkspacesClient{
        deleteErr: errors.New("workspace has active runs"),
    }
    forceFlag := true

    cmd := &WorkspacesDeleteCmd{
        ID:            "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return fakeClient, nil
        },
        prompter:  &wsFailingPrompter{},
        forceFlag: &forceFlag,
    }

    cli := &CLI{Force: true}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to delete workspace") {
        t.Errorf("expected delete failure message, got: %v", err)
    }
}
```

---

### 8. Missing Test: Get Generic API Error (non-404)

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces_test.go`

**Problem:** `TestWorkspacesGet_NotFound` tests 404 errors, but no test covers other API errors (e.g., 403 forbidden, 500 server error).

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspacesGet_APIError` exists and verifies that when the ReadByID API call fails with a non-404 error, it is wrapped and surfaced as a RuntimeError with message "failed to get workspace"
- Verification: Run `go test -v -run "TestWorkspacesGet_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspacesGet_APIError` to `workspaces_test.go` after `TestWorkspacesGet_NotFound`
  2. Use `fakeWorkspacesClient.readErr` to simulate API failure (generic error, not ErrResourceNotFound)
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces_test.go:860-894` - Added `TestWorkspacesGet_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache)
- `make build` - passed (with temp cache)
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesGet_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspacesGet_APIError tests non-404 API errors are surfaced.
func TestWorkspacesGet_APIError(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeWorkspacesClient{
        readErr: errors.New("forbidden"),
    }

    cmd := &WorkspacesGetCmd{
        ID:            "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to get workspace") {
        t.Errorf("expected get failure message, got: %v", err)
    }
}
```

---

## New Features

### 9. Add `--project` Flag to List Command

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces.go`

**Rationale:** The TFC API supports filtering workspaces by project ID via `WorkspaceListOptions.ProjectID`. Organizations with many workspaces benefit from scoped listing. The create command already accepts `--project-id`, so `list --project` is a natural complement.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc workspaces list --project prj-123` passes the project ID to the API via `WorkspaceListOptions.ProjectID`
- Verification: Add test `TestWorkspacesList_WithProjectFilter` that verifies the list options include the project ID
- Implementation:
  1. Add `ProjectID` field to `WorkspacesListCmd` struct with kong tag `name:"project" help:"Filter workspaces by project ID."`
  2. Update `Run()` to build `*tfe.WorkspaceListOptions` when `ProjectID` is set
  3. Pass the options to `client.List()`
  4. Add test case
  5. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces.go:148` - Added `ProjectID string` field with kong tag `name:"project" help:"Filter workspaces by project ID."`
- `cmd/tfc/workspaces.go:188-192` - Added list options building: creates `*tfe.WorkspaceListOptions` when `ProjectID` is set
- `cmd/tfc/workspaces_test.go:375-410` - Added `TestWorkspacesList_WithProjectFilter` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with cache warnings)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesList_WithProjectFilter" ./cmd/tfc/...` - pass

**Implementation:**

1. Add field to `WorkspacesListCmd` struct (around line 148):
```go
type WorkspacesListCmd struct {
    ProjectID string `name:"project" help:"Filter workspaces by project ID."`

    // Dependencies for testing
    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        io.Writer
    clientFactory workspacesClientFactory
}
```

2. Update the `Run` method to pass options (around line 184-185):
```go
// Build list options
var listOpts *tfe.WorkspaceListOptions
if c.ProjectID != "" {
    listOpts = &tfe.WorkspaceListOptions{ProjectID: c.ProjectID}
}

workspaces, err := client.List(ctx, org, listOpts)
```

3. Add test `TestWorkspacesList_WithProjectFilter`:
```go
func TestWorkspacesList_WithProjectFilter(t *testing.T) {
    tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeWorkspacesClient{
        workspaces: []*tfe.Workspace{
            {ID: "ws-1", Name: "workspace-1", ExecutionMode: "remote", Project: &tfe.Project{ID: "prj-123"}},
        },
    }

    cmd := &WorkspacesListCmd{
        ProjectID:     "prj-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify the list was called with project filter
    if len(fakeClient.listCalls) != 1 {
        t.Errorf("expected 1 list call, got %d", len(fakeClient.listCalls))
    }
    if fakeClient.listCalls[0].opts == nil || fakeClient.listCalls[0].opts.ProjectID != "prj-123" {
        t.Errorf("expected project filter prj-123, got: %+v", fakeClient.listCalls[0].opts)
    }
}
```

---

### 10. Add `--search` Flag to List Command

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces.go`

**Rationale:** The TFC API supports name search via `WorkspaceListOptions.Search`. Useful for finding workspaces by partial name match.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc workspaces list --search myws` passes the search string to the API via `WorkspaceListOptions.Search`
- Verification: Add test `TestWorkspacesList_WithSearch` that verifies the list options include the search string
- Implementation:
  1. Add `Search` field to `WorkspacesListCmd` struct with kong tag `name:"search" help:"Search workspaces by name (partial match)."`
  2. Update `Run()` to include Search in `*tfe.WorkspaceListOptions` when set
  3. Add test `TestWorkspacesList_WithSearch`
  4. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces.go:150` - Added `Search string` field with kong tag `name:"search" help:"Search workspaces by name (partial match)."`
- `cmd/tfc/workspaces.go:190-198` - Updated list options building to include Search when set
- `cmd/tfc/workspaces_test.go:412-449` - Added `TestWorkspacesList_WithSearch` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesList_WithSearch" ./cmd/tfc/...` - pass

**Implementation:**

1. Add field to `WorkspacesListCmd` struct:
```go
type WorkspacesListCmd struct {
    ProjectID string `name:"project" help:"Filter workspaces by project ID."`
    Search    string `name:"search" help:"Search workspaces by name (partial match)."`
    // ...
}
```

2. Update the options building:
```go
var listOpts *tfe.WorkspaceListOptions
if c.ProjectID != "" || c.Search != "" {
    listOpts = &tfe.WorkspaceListOptions{}
    if c.ProjectID != "" {
        listOpts.ProjectID = c.ProjectID
    }
    if c.Search != "" {
        listOpts.Search = c.Search
    }
}
```

3. Add test `TestWorkspacesList_WithSearch`.

---

### 11. Add `--tags` Flag to List Command

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspaces.go`

**Rationale:** The TFC API supports filtering by tags via `WorkspaceListOptions.Tags`. Tags are commonly used to organize workspaces.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc workspaces list --tags env:prod,team:platform` passes the tags string to the API via `WorkspaceListOptions.Tags`
- Verification: Add test `TestWorkspacesList_WithTags` that verifies the list options include the tags string
- Implementation:
  1. Add `Tags` field to `WorkspacesListCmd` struct with kong tag `name:"tags" help:"Filter workspaces by tags (comma-separated)."`
  2. Update `Run()` to include Tags in `*tfe.WorkspaceListOptions` when set
  3. Add test `TestWorkspacesList_WithTags`
  4. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspaces.go:150` - Added `Tags string` field with kong tag `name:"tags" help:"Filter workspaces by tags (comma-separated)."`
- `cmd/tfc/workspaces.go:192` - Updated list options condition to include `c.Tags`
- `cmd/tfc/workspaces.go:200-202` - Added Tags assignment to list options when set
- `cmd/tfc/workspaces_test.go:449-485` - Added `TestWorkspacesList_WithTags` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache)
- `make build` - passed (with temp cache)
- `make test` - all tests pass
- `go test -v -run "TestWorkspacesList_WithTags" ./cmd/tfc/...` - pass

**Implementation:**

1. Add field to `WorkspacesListCmd` struct:
```go
type WorkspacesListCmd struct {
    ProjectID string `name:"project" help:"Filter workspaces by project ID."`
    Search    string `name:"search" help:"Search workspaces by name (partial match)."`
    Tags      string `name:"tags" help:"Filter workspaces by tags (comma-separated)."`
    // ...
}
```

2. Update the options building:
```go
if c.Tags != "" {
    listOpts.Tags = c.Tags
}
```

3. Add test `TestWorkspacesList_WithTags`.

---

## Code Quality Improvements

### 12. Extract Duplicate `resolveClientConfig` Function

**Files:** `cmd/tfc/workspaces.go:107-145` and `cmd/tfc/projects.go:112-150`

**Problem:** `resolveWorkspacesClientConfig` and `resolveProjectsClientConfig` are identical functions. This violates DRY and increases maintenance burden.

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

### 13. Extract Duplicate Test Helper Types

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

### 14. Reuse `resolveFormat` Helper

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

## Workspaces Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| List with default org | ✅ | - |
| List with --org override | ✅ | - |
| List fails when no org | ✅ | - |
| List JSON output | ✅ | - |
| List table output | ✅ | - |
| List API error | ✅ | - |
| List missing settings | ✅ | - |
| List with --project filter | ✅ | #9 |
| List with --search filter | ✅ | #10 |
| List with --tags filter | ✅ | #11 |
| Get JSON output | ✅ | - |
| Get not found (404) | ✅ | - |
| Get generic API error | ✅ | #8 |
| Create JSON | ✅ | - |
| Create with project-id | ✅ | - |
| Create no org | ✅ | - |
| Create table output | ✅ | - |
| Create API error | ✅ | #5 |
| Update JSON | ✅ | - |
| Update no fields provided | ✅ | #2 |
| Update API error | ✅ | #6 |
| Delete with confirmation | ✅ | - |
| Delete rejected | ✅ | - |
| Delete with --force | ✅ | - |
| Delete JSON | ✅ | - |
| Delete API error | ✅ | #7 |
| Delete prompter error | ✅ | #4 |

---

# Workspace-Variables Subcommand Code Review

Review of `cmd/tfc/workspace_variables.go` and `cmd/tfc/workspace_variables_test.go`.

---

## Edge Cases

### 15. Update Command Allows No-Op API Calls

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables.go:345-360`

**Problem:** Unlike `ProjectsUpdateCmd` (see `projects.go:386-388`), the workspace-variables update command doesn't validate that at least one field is being updated. If a user runs `tfc workspace-variables update var-123 --workspace-id ws-123` with no other flags, it sends a no-op API call and prints "Variable updated" even though nothing changed.

**Current code:**
```go
opts := tfe.VariableUpdateOptions{}
if c.Key != "" {
    opts.Key = tfe.String(c.Key)
}
if c.Value != "" {
    opts.Value = tfe.String(c.Value)
}
if c.Description != "" {
    opts.Description = tfe.String(c.Description)
}
if c.Sensitive != nil {
    opts.Sensitive = c.Sensitive
}
if c.HCL != nil {
    opts.HCL = c.HCL
}
```

**Fix:** Add validation before the API call (after line 344, before building opts):
```go
// Validate at least one field is being updated
if c.Key == "" && c.Value == "" && c.Description == "" && c.Sensitive == nil && c.HCL == nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("at least one of --key, --value, --description, --sensitive, or --hcl is required"))
}
```

**Test to add:** `TestWorkspaceVariablesUpdate_FailsWhenNoFields` - verify error message when no update flags provided.

**Progress notes (2026-01-21):**

Plan:
- Acceptance criteria: `tfc workspace-variables update var-123 --workspace-id ws-123` (no other flags) returns RuntimeError with message "at least one of --key, --value, --description, --sensitive, or --hcl is required"
- Verification: Add test `TestWorkspaceVariablesUpdate_FailsWhenNoFields` that verifies error is returned and is RuntimeError type
- Implementation:
  1. Add validation check after defaults setup in `WorkspaceVariablesUpdateCmd.Run()`
  2. Add test case in `workspace_variables_test.go`
  3. Run feedback loops

Changes made:
- `cmd/tfc/workspace_variables.go:333-336` - Added validation before config resolution
- `cmd/tfc/workspace_variables_test.go:15` - Added import for `internalcmd`
- `cmd/tfc/workspace_variables_test.go:674-711` - Added `TestWorkspaceVariablesUpdate_FailsWhenNoFields` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceVariablesUpdate_FailsWhenNoFields" ./cmd/tfc/...` - pass

---

### 16. Value Cannot Be Cleared

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

### 17. Description Cannot Be Cleared

**File:** `cmd/tfc/workspace_variables.go:352-354`

**Problem:** Same issue as #16 and the workspaces command - users cannot clear a variable description by passing `--description ""`.

**Current code:**
```go
if c.Description != "" {
    opts.Description = tfe.String(c.Description)
}
```

**Fix:** Same approach as chosen for #16.

---

## Missing Unit Tests

### 18. Missing Test: Prompter Error Path for Delete

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables_test.go`

**Problem:** No test verifies error handling when `prompter.Confirm()` returns an error. The shared `errorPrompter` type already exists in `testhelpers_test.go`.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspaceVariablesDelete_PrompterError` exists and verifies that when prompter.Confirm() returns an error, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for confirmation"
- Verification: Run `go test -v -run "TestWorkspaceVariablesDelete_PrompterError" ./cmd/tfc/...`
- Implementation:
  1. Use the existing shared `errorPrompter` from `testhelpers_test.go`
  2. Add test `TestWorkspaceVariablesDelete_PrompterError` to `workspace_variables_test.go`
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspace_variables_test.go:713-746` - Added `TestWorkspaceVariablesDelete_PrompterError` test using shared `errorPrompter`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceVariablesDelete_PrompterError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspaceVariablesDelete_PrompterError tests that prompter errors are surfaced.
func TestWorkspaceVariablesDelete_PrompterError(t *testing.T) {
    tmpDir, resolver := setupVariablesTestSettings(t)
    var buf bytes.Buffer

    cmd := &WorkspaceVariablesDeleteCmd{
        VariableID:    "var-123",
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
            return &fakeVariablesClient{}, nil
        },
        prompter: &errorPrompter{err: errors.New("terminal not available")},
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
        t.Errorf("expected prompt error, got: %v", err)
    }
}
```

**Note:** Replace `varsAcceptingPrompter` and `varsRejectingPrompter` with the shared types from `testhelpers_test.go` (`acceptingPrompter`, `rejectingPrompter`).

---

### 19. Missing Test: Create API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables_test.go`

**Problem:** No test verifies error handling when the Create API call fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspaceVariablesCreate_APIError` exists and verifies that when the Create API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to create variable"
- Verification: Run `go test -v -run "TestWorkspaceVariablesCreate_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspaceVariablesCreate_APIError` to `workspace_variables_test.go` after `TestWorkspaceVariablesCreate_TerraformCategory`
  2. Verify test passes with `make test`

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspace_variables_test.go:638-680` - Added `TestWorkspaceVariablesCreate_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with cache warnings)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceVariablesCreate_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspaceVariablesCreate_APIError tests that API errors are surfaced during create.
func TestWorkspaceVariablesCreate_APIError(t *testing.T) {
    tmpDir, resolver := setupVariablesTestSettings(t)
    var buf bytes.Buffer

    fakeClient := &fakeVariablesClient{
        createErr: errors.New("variable key already exists"),
    }

    cmd := &WorkspaceVariablesCreateCmd{
        WorkspaceID:   "ws-123",
        Key:           "EXISTING_VAR",
        Value:         "some-value",
        Category:      "env",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to create variable") {
        t.Errorf("expected create failure message, got: %v", err)
    }
}
```

---

### 20. Missing Test: Update API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables_test.go`

**Problem:** No test verifies error handling when the Update API call fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspaceVariablesUpdate_APIError` exists and verifies that when the Update API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to update variable"
- Verification: Run `go test -v -run "TestWorkspaceVariablesUpdate_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspaceVariablesUpdate_APIError` to `workspace_variables_test.go` after `TestWorkspaceVariablesDelete_PrompterError`
  2. Use `fakeVariablesClient.updateErr` to simulate API failure
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspace_variables_test.go:788-822` - Added `TestWorkspaceVariablesUpdate_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceVariablesUpdate_APIError" ./cmd/tfc/...` - pass

---

### 21. Missing Test: Delete API Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables_test.go`

**Problem:** No test verifies error handling when the Delete API call fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestWorkspaceVariablesDelete_APIError` exists and verifies that when the Delete API call fails, it is wrapped and surfaced as a RuntimeError with message "failed to delete variable"
- Verification: Run `go test -v -run "TestWorkspaceVariablesDelete_APIError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestWorkspaceVariablesDelete_APIError` to `workspace_variables_test.go` after `TestWorkspaceVariablesUpdate_APIError`
  2. Use `fakeVariablesClient.deleteErr` to simulate API failure
  3. Use `--force` flag to skip confirmation and test API error path directly
  4. Verify RuntimeError type for exit code 2
  5. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspace_variables_test.go:826-860` - Added `TestWorkspaceVariablesDelete_APIError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache due to permission issues)
- `make build` - passed (with temp cache)
- `make test` - all tests pass (with temp cache)
- `go test -v -run "TestWorkspaceVariablesDelete_APIError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestWorkspaceVariablesDelete_APIError tests that API errors are surfaced during delete.
func TestWorkspaceVariablesDelete_APIError(t *testing.T) {
    tmpDir, resolver := setupVariablesTestSettings(t)
    var buf bytes.Buffer

    fakeClient := &fakeVariablesClient{
        deleteErr: errors.New("forbidden"),
    }
    forceTrue := true

    cmd := &WorkspaceVariablesDeleteCmd{
        VariableID:    "var-123",
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
            return fakeClient, nil
        },
        forceFlag: &forceTrue,
    }

    cli := &CLI{Force: true}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to delete variable") {
        t.Errorf("expected delete failure message, got: %v", err)
    }
}
```

---

### 22. Missing Test: Update with No Fields Provided

**Status: DONE** (2026-01-21) - Completed as part of task #15

**File:** `cmd/tfc/workspace_variables_test.go`

**Problem:** No test verifies behavior when update is called without any fields to update. This test should be added after fixing issue #15.

**Test to add (after #15 is fixed):**
```go
// TestWorkspaceVariablesUpdate_FailsWhenNoFields tests that update fails when no fields provided.
func TestWorkspaceVariablesUpdate_FailsWhenNoFields(t *testing.T) {
    tmpDir, resolver := setupVariablesTestSettings(t)
    var buf bytes.Buffer

    cmd := &WorkspaceVariablesUpdateCmd{
        VariableID:    "var-123",
        WorkspaceID:   "ws-123",
        // No update fields provided
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
            return &fakeVariablesClient{}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when no fields provided, got nil")
    }
    if !strings.Contains(err.Error(), "at least one of") {
        t.Errorf("expected 'at least one of' error message, got: %v", err)
    }
}
```

---

## Missing Features

### 23. Missing Get Subcommand

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_variables.go`

**Problem:** The `variablesClient` interface defines a `Read` method (line 68), but there is no `WorkspaceVariablesGetCmd` to retrieve a single variable by ID. This is inconsistent with the workspaces subcommand which has List/Get/Create/Update/Delete.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc workspace-variables get VAR_ID --workspace-id WS_ID` retrieves and displays a single variable by ID
- Verification: Add tests `TestWorkspaceVariablesGet_JSON`, `TestWorkspaceVariablesGet_Table`, `TestWorkspaceVariablesGet_NotFound`, `TestWorkspaceVariablesGet_APIError`
- Implementation:
  1. Add `Get` to `WorkspaceVariablesCmd` struct
  2. Add `WorkspaceVariablesGetCmd` struct with `VariableID` (arg) and `WorkspaceID` (flag)
  3. Implement `Run()` method following `WorkspacesGetCmd` pattern
  4. Add table output showing FIELD/VALUE pairs for variable details
  5. Add tests for JSON, Table, NotFound, and API error cases
  6. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/workspace_variables.go:21` - Added `Get WorkspaceVariablesGetCmd` to command group
- `cmd/tfc/workspace_variables.go:214-287` - Added `WorkspaceVariablesGetCmd` struct and `Run()` method
- `cmd/tfc/workspace_variables_test.go:265-431` - Added 4 tests: `TestWorkspaceVariablesGet_JSON`, `TestWorkspaceVariablesGet_Table`, `TestWorkspaceVariablesGet_NotFound`, `TestWorkspaceVariablesGet_APIError`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceVariablesGet" ./cmd/tfc/...` - all 4 tests pass

**Impact:** Users must use `list` and filter client-side to find a specific variable's details.

**Implementation:**

1. Add new command struct:
```go
// WorkspaceVariablesGetCmd retrieves a single variable.
type WorkspaceVariablesGetCmd struct {
    VariableID  string `arg:"" help:"ID of the variable to retrieve."`
    WorkspaceID string `required:"" name:"workspace-id" help:"ID of the workspace."`

    // Dependencies for testing
    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        io.Writer
    clientFactory variablesClientFactory
}
```

2. Add to command group (line 21-25):
```go
type WorkspaceVariablesCmd struct {
    List   WorkspaceVariablesListCmd   `cmd:"" help:"List variables for a workspace."`
    Get    WorkspaceVariablesGetCmd    `cmd:"" help:"Get a variable by ID."`
    Create WorkspaceVariablesCreateCmd `cmd:"" help:"Create a new variable."`
    Update WorkspaceVariablesUpdateCmd `cmd:"" help:"Update a variable."`
    Delete WorkspaceVariablesDeleteCmd `cmd:"" help:"Delete a variable."`
}
```

3. Implement `Run` method similar to `WorkspacesGetCmd`.

4. Add tests: `TestWorkspaceVariablesGet_JSON`, `TestWorkspaceVariablesGet_Table`, `TestWorkspaceVariablesGet_NotFound`, `TestWorkspaceVariablesGet_APIError`.

---

## Code Quality Improvements

### 24. Duplicate `resolveVariablesClientConfig` Function

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

### 25. Duplicate Test Helper Types

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

### 26. Inline TTY Detection Pattern

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

**Fix:** Use a shared helper function as suggested in finding #14. After creating the shared helper, update all 4 locations in workspace_variables.go.

---

## Workspace-Variables Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| List JSON output | ✅ | - |
| List table output | ✅ | - |
| List API error | ✅ | - |
| List missing settings | ✅ | - |
| Get by ID | ✅ | #23 |
| Get table output | ✅ | #23 |
| Get not found (404) | ✅ | #23 |
| Get generic API error | ✅ | #23 |
| Create JSON output | ✅ | - |
| Create table output | ✅ | - |
| Create with sensitive/HCL | ✅ | - |
| Create terraform category | ✅ | - |
| Create API error | ✅ | #19 |
| Update JSON output | ✅ | - |
| Update partial (only value) | ✅ | - |
| Update no fields provided | ✅ | #22 |
| Update API error | ✅ | #20 |
| Delete with confirmation | ✅ | - |
| Delete rejected | ✅ | - |
| Delete with --force | ✅ | - |
| Delete JSON output | ✅ | - |
| Delete API error | ✅ | #21 |
| Delete prompter error | ✅ | #18 |

---

# Init Subcommand Code Review

Review of `cmd/tfc/main.go` (InitCmd) and `cmd/tfc/init_test.go`.

---

## Bugs

### 27. Missing Address Validation

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/main.go:321` (interactive) and `305-308` (non-interactive)

**Problem:** The init command accepts any address value without validation. Unlike the `doctor` command which validates the address format using `auth.ExtractHostname()` (main.go:158), the init command saves whatever address is provided. This allows invalid addresses (e.g., "not-a-url", "://broken") to be stored in settings.json, causing confusing failures later when running other commands.

**Current code:**
```go
address, err = c.prompter.PromptString("API address", defaultAddress)
if err != nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for address: %w", err))
}
// No validation of address format
```

**Fix:** Add address validation after collecting the value (both interactive and non-interactive paths):
```go
address, err = c.prompter.PromptString("API address", defaultAddress)
if err != nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("failed to prompt for address: %w", err))
}

// Validate address format
if _, err := auth.ExtractHostname(address); err != nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("invalid address %q: %w", address, err))
}
```

**Test to add:** `TestInitCmd_InvalidAddressRejected` - verify error when address format is invalid.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc init --non-interactive --address "://broken"` returns RuntimeError with message containing "invalid address"
- Verification: Add test `TestInitCmd_InvalidAddressRejected` that verifies error is returned and is RuntimeError type
- Implementation:
  1. Add validation using `auth.ExtractHostname()` after address is collected (both interactive and non-interactive paths)
  2. Add test case in `init_test.go`
  3. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/main.go:338-341` - Added address validation using `auth.ExtractHostname()` after collecting address value (before creating settings)
- `cmd/tfc/init_test.go:330-375` - Added `TestInitCmd_InvalidAddressRejected` test with subtests for malformed URL and empty hostname cases

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_InvalidAddressRejected" ./cmd/tfc/...` - pass (both subtests pass)

---

### 28. os.Stat Error Handling Ignores Permission Errors

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/main.go:277-280`

**Problem:** The code checks if settings exist using `os.Stat`, but doesn't distinguish between "file doesn't exist" (`os.ErrNotExist`) and other errors like permission denied. If `os.Stat` returns a permission error, it's treated as "file doesn't exist" leading the code to try creating a new file, which may also fail with a confusing error.

**Current code:**
```go
settingsExist := false
if _, err := os.Stat(settingsPath); err == nil {
    settingsExist = true
}
```

**Fix:** Handle the error case explicitly:
```go
settingsExist := false
if _, err := os.Stat(settingsPath); err == nil {
    settingsExist = true
} else if !errors.Is(err, os.ErrNotExist) {
    // Permission error or other unexpected error
    return internalcmd.NewRuntimeError(fmt.Errorf("failed to check settings file: %w", err))
}
```

**Plan (2026-01-21):**
- Acceptance criteria: `tfc init` returns RuntimeError with "failed to check settings file" when os.Stat encounters a permission error (or other non-ErrNotExist error)
- Verification: Add test `TestInitCmd_StatPermissionError` that verifies error is returned and is RuntimeError type
- Implementation:
  1. Edit `cmd/tfc/main.go:277-280` to check for errors other than `os.ErrNotExist`
  2. Add test case in `init_test.go`
  3. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/main.go:277-281` - Added `else if !errors.Is(err, os.ErrNotExist)` check to return RuntimeError for permission or other unexpected errors
- `cmd/tfc/init_test.go:376-427` - Added `TestInitCmd_StatPermissionError` test that creates settings with no read permission and verifies RuntimeError is returned with "failed to check settings file" message

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_StatPermissionError" ./cmd/tfc/...` - pass

---

## Edge Cases

### 29. Console Output Not Testable

**File:** `cmd/tfc/main.go:290, 355`

**Problem:** Unlike other commands (e.g., `DoctorCmd`, `WorkspacesListCmd`) that use an injectable `stdout` writer, `InitCmd` writes directly to `os.Stdout` via `fmt.Println()` and `fmt.Printf()`. This makes output untestable and inconsistent with the rest of the codebase.

**Current code:**
```go
fmt.Println("Aborting init (settings unchanged).")
// ...
fmt.Printf("Settings written to %s\n", settingsPath)
```

**Fix:**

1. Add `stdout` field to `InitCmd`:
```go
type InitCmd struct {
    NonInteractive bool   `help:"Run in non-interactive mode (for CI/agents)."`
    DefaultOrg     string `name:"default-org" help:"Default organization."`
    LogLevel       string `name:"log-level" enum:"debug,info,warn,error," default:"" help:"Log level (debug, info, warn, error)."`
    Yes            bool   `help:"Skip confirmation prompts (e.g., overwrite existing settings)."`

    // Dependencies (injectable for testing)
    prompter ui.Prompter
    baseDir  string
    stdout   io.Writer  // Add this
}
```

2. Set default in Run():
```go
if c.stdout == nil {
    c.stdout = os.Stdout
}
```

3. Replace `fmt.Println`/`fmt.Printf` with writes to `c.stdout`:
```go
fmt.Fprintln(c.stdout, "Aborting init (settings unchanged).")
fmt.Fprintf(c.stdout, "Settings written to %s\n", settingsPath)
```

---

## Missing Unit Tests

### 30. Missing Test: Prompter Error on Overwrite Confirmation

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `prompter.Confirm()` returns an error during the "overwrite existing settings?" prompt.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestInitCmd_PrompterErrorOnOverwriteConfirm` exists and verifies that when prompter.Confirm() returns an error during overwrite confirmation, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for confirmation"
- Verification: Run `go test -v -run "TestInitCmd_PrompterErrorOnOverwriteConfirm" ./cmd/tfc/...`
- Implementation:
  1. Use the existing shared `errorPrompter` from `testhelpers_test.go`
  2. Add test `TestInitCmd_PrompterErrorOnOverwriteConfirm` to `init_test.go`
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/init_test.go:376-409` - Added `TestInitCmd_PrompterErrorOnOverwriteConfirm` test using shared `errorPrompter`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp cache due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_PrompterErrorOnOverwriteConfirm" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestInitCmd_PrompterErrorOnOverwriteConfirm tests prompter error during overwrite prompt.
func TestInitCmd_PrompterErrorOnOverwriteConfirm(t *testing.T) {
    tmpHome := t.TempDir()

    // Create existing settings
    existingSettings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "existing.example.com"},
        },
    }
    if err := config.Save(existingSettings, tmpHome); err != nil {
        t.Fatalf("Failed to create existing settings: %v", err)
    }

    cmd := &InitCmd{
        prompter: &errorPrompter{err: errors.New("terminal not available")},
        baseDir:  tmpHome,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
        t.Errorf("expected prompt confirmation error, got: %v", err)
    }
}
```

**Note:** Uses `errorPrompter` from `testhelpers_test.go`.

---

### 31. Missing Test: Prompter Error on Address Input

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `prompter.PromptString()` returns an error for the address prompt.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestInitCmd_PrompterErrorOnAddress` exists and verifies that when prompter.PromptString() returns an error during address prompt, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for address"
- Verification: Run `go test -v -run "TestInitCmd_PrompterErrorOnAddress" ./cmd/tfc/...`
- Implementation:
  1. Use the existing shared `errorPrompter` from `testhelpers_test.go`
  2. Add test `TestInitCmd_PrompterErrorOnAddress` to `init_test.go`
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/init_test.go:413-437` - Added `TestInitCmd_PrompterErrorOnAddress` test using shared `errorPrompter`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_PrompterErrorOnAddress" ./cmd/tfc/...` - pass

---

### 32. Missing Test: Prompter Error on Organization Input

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `prompter.PromptString()` returns an error for the organization prompt. This requires a custom prompter that succeeds on the first call (address) but fails on the second (org).

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestInitCmd_PrompterErrorOnOrg` exists and verifies that when prompter.PromptString() returns an error during the org prompt, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for default org"
- Verification: Run `go test -v -run "TestInitCmd_PrompterErrorOnOrg" ./cmd/tfc/...`
- Implementation:
  1. Add `sequentialErrorPrompter` type to `testhelpers_test.go` (succeeds on first N calls, then fails)
  2. Add test `TestInitCmd_PrompterErrorOnOrg` to `init_test.go`
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/testhelpers_test.go:67-87` - Added `sequentialErrorPrompter` type that returns default values until reaching errorOnCall, then returns error
- `cmd/tfc/init_test.go:439-462` - Added `TestInitCmd_PrompterErrorOnOrg` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with cache warnings due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_PrompterErrorOnOrg" ./cmd/tfc/...` - pass

**Test to add:**
```go
// sequentialErrorPrompter returns values until it reaches the error call.
type sequentialErrorPrompter struct {
    stringCalls int
    errorOnCall int
    err         error
}

func (p *sequentialErrorPrompter) PromptString(_, defaultValue string) (string, error) {
    p.stringCalls++
    if p.stringCalls == p.errorOnCall {
        return "", p.err
    }
    return defaultValue, nil
}

func (p *sequentialErrorPrompter) Confirm(_ string, defaultValue bool) (bool, error) {
    return defaultValue, nil
}

func (p *sequentialErrorPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
    return defaultValue, nil
}

// TestInitCmd_PrompterErrorOnOrg tests prompter error during org prompt.
func TestInitCmd_PrompterErrorOnOrg(t *testing.T) {
    tmpHome := t.TempDir()

    cmd := &InitCmd{
        prompter: &sequentialErrorPrompter{errorOnCall: 2, err: errors.New("EOF")},
        baseDir:  tmpHome,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for default org") {
        t.Errorf("expected org prompt error, got: %v", err)
    }
}
```

---

### 33. Missing Test: Prompter Error on Log Level Selection

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `prompter.PromptSelect()` returns an error for the log level prompt.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestInitCmd_PrompterErrorOnLogLevel` exists and verifies that when prompter.PromptSelect() returns an error during log level prompt, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for log level"
- Verification: Run `go test -v -run "TestInitCmd_PrompterErrorOnLogLevel" ./cmd/tfc/...`
- Implementation:
  1. Add `selectErrorPrompter` type to `testhelpers_test.go` (succeeds on string prompts and confirms, fails on select)
  2. Add test `TestInitCmd_PrompterErrorOnLogLevel` to `init_test.go`
  3. Verify RuntimeError type for exit code 2
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/testhelpers_test.go:92-107` - Added `selectErrorPrompter` type that succeeds on PromptString and Confirm but fails on PromptSelect
- `cmd/tfc/init_test.go:466-489` - Added `TestInitCmd_PrompterErrorOnLogLevel` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_PrompterErrorOnLogLevel" ./cmd/tfc/...` - pass

**Test to add:**
```go
// selectErrorPrompter succeeds on string prompts but fails on select.
type selectErrorPrompter struct {
    err error
}

func (p *selectErrorPrompter) PromptString(_, defaultValue string) (string, error) {
    return defaultValue, nil
}

func (p *selectErrorPrompter) Confirm(_ string, defaultValue bool) (bool, error) {
    return defaultValue, nil
}

func (p *selectErrorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", p.err
}

// TestInitCmd_PrompterErrorOnLogLevel tests prompter error during log level prompt.
func TestInitCmd_PrompterErrorOnLogLevel(t *testing.T) {
    tmpHome := t.TempDir()

    cmd := &InitCmd{
        prompter: &selectErrorPrompter{err: errors.New("EOF")},
        baseDir:  tmpHome,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for log level") {
        t.Errorf("expected log level prompt error, got: %v", err)
    }
}
```

---

### 34. Missing Test: config.Save Failure

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `config.Save()` fails (e.g., disk full, read-only filesystem).

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestInitCmd_SaveError` exists and verifies that when config.Save() fails (e.g., read-only directory), it is wrapped and surfaced as a RuntimeError with message "failed to save settings"
- Verification: Run `go test -v -run "TestInitCmd_SaveError" ./cmd/tfc/...`
- Implementation:
  1. Add test that creates a temp directory with settings dir made read-only
  2. Run init command in non-interactive mode with --yes
  3. Verify RuntimeError is returned with "failed to save settings" message

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/init_test.go:492-528` - Added `TestInitCmd_SaveError` test that creates a read-only .tfccli directory and verifies the proper RuntimeError is returned

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestInitCmd_SaveError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestInitCmd_SaveError tests that save errors are properly surfaced.
func TestInitCmd_SaveError(t *testing.T) {
    // Create a temp dir and then make it read-only
    tmpHome := t.TempDir()
    tfccliDir := filepath.Join(tmpHome, ".tfccli")
    if err := os.MkdirAll(tfccliDir, 0o700); err != nil {
        t.Fatalf("Failed to create dir: %v", err)
    }
    // Make directory read-only to prevent writing
    if err := os.Chmod(tfccliDir, 0o500); err != nil {
        t.Fatalf("Failed to chmod: %v", err)
    }
    t.Cleanup(func() {
        os.Chmod(tfccliDir, 0o700) // Restore so cleanup can delete
    })

    cmd := &InitCmd{
        NonInteractive: true,
        Yes:            true,
        baseDir:        tmpHome,
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

### 35. Missing Test: SettingsPath Error

**File:** `cmd/tfc/init_test.go`

**Problem:** No test verifies error handling when `config.SettingsPath()` fails. This is difficult to trigger since `SettingsPath` only fails if `os.UserHomeDir()` fails and no `baseDir` is provided. Consider adding a test with a mock or skip if not feasible.

**Note:** This is a low-priority test since the error path is unlikely in practice (only fails if HOME env var is unset and no baseDir override). The error is properly wrapped with RuntimeError at line 273-274.

---

## Init Command Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| Create settings with defaults (interactive) | ✅ | - |
| Non-interactive with provided values | ✅ | - |
| Non-interactive uses defaults | ✅ | - |
| Interactive reject overwrite | ✅ | - |
| Interactive accept overwrite | ✅ | - |
| Non-interactive with --yes overwrites | ✅ | - |
| Non-interactive without --yes errors on existing | ✅ | - |
| Interactive with custom values | ✅ | - |
| Invalid address format rejected | ✅ | #27 |
| Prompter error on overwrite confirm | ✅ | #30 |
| Prompter error on address prompt | ✅ | #31 |
| Prompter error on org prompt | ✅ | #32 |
| Prompter error on log level prompt | ✅ | #33 |
| config.Save() failure | ✅ | #34 |
| os.Stat permission error handling | ✅ | #28 |

---

# Doctor Subcommand Code Review

Review of `cmd/tfc/main.go` (DoctorCmd, lines 64-248) and `cmd/tfc/doctor_test.go`.

---

## Bugs

### 36. stdout Field Type Inconsistency

**File:** `cmd/tfc/main.go:70`

**Problem:** `DoctorCmd.stdout` is typed as `*os.File`, while other commands (e.g., `ProjectsListCmd`, `WorkspacesListCmd`) use `io.Writer`. This inconsistency:
1. Limits testing flexibility - can only inject `*os.File`, not arbitrary writers like `bytes.Buffer`
2. Breaks the pattern established by other commands
3. Makes the `ttyDetector.IsTTY()` call on line 106 work directly, but at the cost of the abstraction

**Current code:**
```go
type DoctorCmd struct {
    // Dependencies for testing
    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        *os.File  // Should be io.Writer
    clientFactory func(cfg tfcapi.ClientConfig) (doctorClient, error)
}
```

**Fix:** Change to `io.Writer` and use type assertion pattern like other commands:

```go
type DoctorCmd struct {
    // Dependencies for testing
    baseDir       string
    tokenResolver *auth.TokenResolver
    ttyDetector   output.TTYDetector
    stdout        io.Writer  // Changed from *os.File
    clientFactory func(cfg tfcapi.ClientConfig) (doctorClient, error)
}

func (d *DoctorCmd) Run(cli *CLI) error {
    // ... existing setup ...

    if d.stdout == nil {
        d.stdout = os.Stdout
    }

    // Use type assertion for TTY detection
    isTTY := false
    if f, ok := d.stdout.(*os.File); ok {
        isTTY = d.ttyDetector.IsTTY(f)
    }
    // ... rest of function
}
```

**Impact:** This also affects `outputAndError()` which currently passes `d.stdout` to `output.WriteJSON()` and `output.NewTableWriter()` - both accept `io.Writer`, so no changes needed there.

---

## Edge Cases

### 37. Context Not Found Error Message Lacks Guidance

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

---

## Missing Unit Tests

### 38. Missing Test: Context Not Found with --context Flag

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/doctor_test.go`

**Problem:** No test verifies the behavior when `--context` flag specifies a non-existent context name. The existing tests use valid context names or rely on `CurrentContext`.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestDoctor_ContextNotFound` exists and verifies that when `--context` flag specifies a non-existent context, the doctor command fails with context check status FAIL and detail containing the context name
- Verification: Run `go test -v -run "TestDoctor_ContextNotFound" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestDoctor_ContextNotFound` to `doctor_test.go`
  2. Create settings with only "default" context
  3. Use `--context nonexistent` to trigger the error path
  4. Verify RuntimeError is returned and context check shows FAIL status

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/doctor_test.go:662-711` - Added `TestDoctor_ContextNotFound` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestDoctor_ContextNotFound" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestDoctor_ContextNotFound tests error when --context flag specifies non-existent context.
func TestDoctor_ContextNotFound(t *testing.T) {
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
        t.Fatalf("failed to save settings: %v", err)
    }

    stdout, getOutput := captureStdout(t)

    cmd := &DoctorCmd{
        baseDir:     tmpDir,
        stdout:      stdout,
        ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
    }
    cli := &CLI{OutputFormat: "json", Context: "nonexistent"}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    out := getOutput()

    var result DoctorResult
    if err := json.Unmarshal([]byte(out), &result); err != nil {
        t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
    }

    ctxCheck := findCheck(result.Checks, "context")
    if ctxCheck == nil {
        t.Fatal("context check not found")
    }

    if ctxCheck.Status != "FAIL" {
        t.Errorf("expected context status FAIL, got: %s", ctxCheck.Status)
    }
    if !strings.Contains(ctxCheck.Detail, "nonexistent") {
        t.Errorf("expected detail to contain 'nonexistent', got: %s", ctxCheck.Detail)
    }
}
```

---

### 39. Missing Test: Invalid Address Format

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/doctor_test.go`

**Problem:** No test verifies the behavior when the address in the context is malformed and `auth.ExtractHostname()` fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestDoctor_InvalidAddressFormat` exists and verifies that when the address in the context is malformed, the doctor command fails with address check status FAIL and detail containing "invalid address"
- Verification: Run `go test -v -run "TestDoctor_InvalidAddressFormat" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestDoctor_InvalidAddressFormat` to `doctor_test.go`
  2. Create settings with malformed address "://invalid-url"
  3. Verify error is returned and address check shows FAIL status

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/doctor_test.go:714-761` - Added `TestDoctor_InvalidAddressFormat` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestDoctor_InvalidAddressFormat" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestDoctor_InvalidAddressFormat tests error when address is malformed.
func TestDoctor_InvalidAddressFormat(t *testing.T) {
    tmpDir := t.TempDir()

    // Create settings with invalid address
    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {
                Address:  "://invalid-url",  // Malformed URL
                LogLevel: "info",
            },
        },
    }
    if err := config.Save(settings, tmpDir); err != nil {
        t.Fatalf("failed to save settings: %v", err)
    }

    stdout, getOutput := captureStdout(t)

    cmd := &DoctorCmd{
        baseDir:     tmpDir,
        stdout:      stdout,
        ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
    }
    cli := &CLI{OutputFormat: "json"}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    out := getOutput()

    var result DoctorResult
    if err := json.Unmarshal([]byte(out), &result); err != nil {
        t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
    }

    addrCheck := findCheck(result.Checks, "address")
    if addrCheck == nil {
        t.Fatal("address check not found")
    }

    if addrCheck.Status != "FAIL" {
        t.Errorf("expected address status FAIL, got: %s", addrCheck.Status)
    }
}
```

---

### 40. Missing Test: Client Factory Returns Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/doctor_test.go`

**Problem:** No test verifies the behavior when `clientFactory` returns an error (distinct from `Ping()` returning an error). This tests lines 192-204.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestDoctor_ClientFactoryError` exists and verifies that when clientFactory returns an error, the doctor command fails with connectivity check status FAIL and detail containing "failed to create client"
- Verification: Run `go test -v -run "TestDoctor_ClientFactoryError" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestDoctor_ClientFactoryError` to `doctor_test.go`
  2. Create settings with valid address and token
  3. Return error from clientFactory to trigger the error path
  4. Verify RuntimeError is returned and connectivity check shows FAIL status with "failed to create client" message

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/doctor_test.go:766-829` - Added `TestDoctor_ClientFactoryError` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestDoctor_ClientFactoryError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestDoctor_ClientFactoryError tests error when client factory fails.
func TestDoctor_ClientFactoryError(t *testing.T) {
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
        t.Fatalf("failed to save settings: %v", err)
    }

    stdout, getOutput := captureStdout(t)

    fakeEnvMap := &fakeEnv{
        vars: map[string]string{
            "TF_TOKEN_app_terraform_io": "fake-token",
        },
    }
    fakeFSMap := &fakeFS{
        homeDir: tmpDir,
        files:   make(map[string][]byte),
    }

    cmd := &DoctorCmd{
        baseDir:       tmpDir,
        stdout:        stdout,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
        clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
            return nil, errors.New("failed to initialize TFC client")
        },
    }
    cli := &CLI{OutputFormat: "json"}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    out := getOutput()

    var result DoctorResult
    if err := json.Unmarshal([]byte(out), &result); err != nil {
        t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
    }

    connCheck := findCheck(result.Checks, "connectivity")
    if connCheck == nil {
        t.Fatal("connectivity check not found")
    }

    if connCheck.Status != "FAIL" {
        t.Errorf("expected connectivity status FAIL, got: %s", connCheck.Status)
    }
    if !strings.Contains(connCheck.Detail, "failed to create client") {
        t.Errorf("expected detail to contain 'failed to create client', got: %s", connCheck.Detail)
    }
}
```

---

### 41. Missing Test: Empty Address in Context

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/doctor_test.go`

**Problem:** No test verifies the behavior when the context has an empty address and relies on `WithDefaults()` to apply `DefaultAddress`. While this path likely works, it's not explicitly tested.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestDoctor_EmptyAddressUsesDefault` exists and verifies that when the context has an empty address, the doctor command uses the default `app.terraform.io` address and passes all checks
- Verification: Run `go test -v -run "TestDoctor_EmptyAddressUsesDefault" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestDoctor_EmptyAddressUsesDefault` to `doctor_test.go`
  2. Create settings with empty address in context
  3. Provide token for default `app.terraform.io` host
  4. Verify address check passes with default hostname in detail

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/doctor_test.go:831-889` - Added `TestDoctor_EmptyAddressUsesDefault` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestDoctor_EmptyAddressUsesDefault" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestDoctor_EmptyAddressUsesDefault tests that empty address in context uses default.
func TestDoctor_EmptyAddressUsesDefault(t *testing.T) {
    tmpDir := t.TempDir()

    // Create settings with empty address (should default to app.terraform.io)
    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {
                Address:  "",  // Empty - should use default
                LogLevel: "info",
            },
        },
    }
    if err := config.Save(settings, tmpDir); err != nil {
        t.Fatalf("failed to save settings: %v", err)
    }

    stdout, getOutput := captureStdout(t)

    fakeEnvMap := &fakeEnv{
        vars: map[string]string{
            "TF_TOKEN_app_terraform_io": "fake-token",
        },
    }
    fakeFSMap := &fakeFS{
        homeDir: tmpDir,
        files:   make(map[string][]byte),
    }

    cmd := &DoctorCmd{
        baseDir:       tmpDir,
        stdout:        stdout,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
        clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
            return &fakeDoctorClient{pingErr: nil}, nil
        },
    }
    cli := &CLI{OutputFormat: "json"}

    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := getOutput()

    var result DoctorResult
    if err := json.Unmarshal([]byte(out), &result); err != nil {
        t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
    }

    // Verify address check shows default hostname
    addrCheck := findCheck(result.Checks, "address")
    if addrCheck == nil {
        t.Fatal("address check not found")
    }

    if !strings.Contains(addrCheck.Detail, "app.terraform.io") {
        t.Errorf("expected address detail to contain default 'app.terraform.io', got: %s", addrCheck.Detail)
    }
}
```

---

## Code Quality Improvements

### 42. Duplicate Test Helper Types

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

**Fix:** Move these types to `cmd/tfc/testhelpers_test.go` and update all test files to use the shared types. This aligns with finding #13 and #25 which identified the same duplication pattern in other test files.

---

### 43. Missing `isTTY` Parameter Consistency

**File:** `cmd/tfc/main.go:106`

**Problem:** The doctor command calculates `isTTY` differently from other commands. It passes `d.stdout` (which is `*os.File`) directly to `IsTTY()`, while other commands use type assertion from `io.Writer`.

**Current code (doctor):**
```go
isTTY := d.ttyDetector.IsTTY(d.stdout)
```

**Pattern in other commands (e.g., projects.go):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
```

**Fix:** After fixing #36 (changing `stdout` to `io.Writer`), use the same type assertion pattern for consistency.

---

### 44. Consider Using `resolveFormat` Helper

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

**Note:** This depends on first moving `resolveFormat` from `projects.go` to a shared location as suggested in finding #14.

---

## Doctor Command Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| Settings file missing | ✅ | - |
| Token from env variable | ✅ | - |
| Token from credentials.tfrc.json | ✅ | - |
| No token found | ✅ | - |
| Connectivity error (Ping fails) | ✅ | - |
| All checks pass | ✅ | - |
| Table output format | ✅ | - |
| JSON output format | ✅ | - |
| --context flag override | ✅ | - |
| --address flag override | ✅ | - |
| Context not found (--context flag) | ✅ | #38 |
| Invalid address format | ✅ | #39 |
| Client factory error | ✅ | #40 |
| Empty address uses default | ✅ | #41 |

---

# Contexts Subcommand Code Review

Review of `cmd/tfc/main.go` (ContextsCmd, lines 359-540) and `cmd/tfc/contexts_test.go`.

---

## Bugs

### 45. No JSON Output Format Support

**File:** `cmd/tfc/main.go:373-387` (list), `511-539` (show)

**Problem:** The contexts commands use `fmt.Printf` directly without supporting `--output-format json/table` like other commands. This is inconsistent with the rest of the codebase (projects, workspaces, doctor, etc.) which all support both JSON and table output.

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

### 46. Console Output Not Testable

**File:** `cmd/tfc/main.go:373-387, 419, 446-447, 489-490, 500, 535-538`

**Problem:** All contexts commands write directly to `os.Stdout` via `fmt.Printf()` and `fmt.Println()`, making output untestable. Other commands (ProjectsListCmd, WorkspacesListCmd, DoctorCmd) have injectable `stdout` fields.

**Current code locations:**
- `ContextsListCmd.Run()` line 384: `fmt.Printf("%s%s\n", marker, name)`
- `ContextsAddCmd.Run()` line 419: `fmt.Printf("Context %q added.\n", c.Name)`
- `ContextsUseCmd.Run()` line 446: `fmt.Printf("Switched to context %q.\n", c.Name)`
- `ContextsRemoveCmd.Run()` line 489: `fmt.Println("Aborting removal.")`
- `ContextsRemoveCmd.Run()` line 500: `fmt.Printf("Context %q removed.\n", c.Name)`
- `ContextsShowCmd.Run()` lines 535-538: multiple `fmt.Printf` calls

**Fix:** Add `stdout io.Writer` field to all contexts command structs and use `fmt.Fprintf(c.stdout, ...)` instead of `fmt.Printf(...)`. Set default in `Run()`:
```go
if c.stdout == nil {
    c.stdout = os.Stdout
}
```

---

### 47. List Output Order is Non-Deterministic

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/main.go:386-392`

**Problem:** The list command iterates over `settings.Contexts` map directly. Map iteration in Go is not deterministic, meaning context listing may appear in different orders between runs. This makes testing difficult and the CLI behavior unpredictable.

**Plan (2026-01-21):**
- Acceptance criteria: `tfc contexts list` outputs context names in alphabetical order, deterministically
- Verification: Run existing tests, add test for alphabetical ordering if not covered
- Implementation:
  1. Import `sort` package in main.go
  2. Collect context names into a slice
  3. Sort the slice alphabetically
  4. Iterate over the sorted slice instead of the map directly

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/main.go:8` - Added `sort` import
- `cmd/tfc/main.go:387-392` - Added slice collection and `sort.Strings(names)` before iteration

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- Output now deterministic with alphabetical ordering

**Current code:**
```go
for name := range settings.Contexts {
    marker := "  "
    if name == settings.CurrentContext {
        marker = "* "
    }
    fmt.Printf("%s%s\n", marker, name)
}
```

**Fix:** Collect keys, sort them, then iterate:
```go
names := make([]string, 0, len(settings.Contexts))
for name := range settings.Contexts {
    names = append(names, name)
}
sort.Strings(names)

for _, name := range names {
    marker := "  "
    if name == settings.CurrentContext {
        marker = "* "
    }
    fmt.Fprintf(c.stdout, "%s%s\n", marker, name)
}
```

---

### 48. Missing Address Validation in Add Command

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/main.go:399-421`

**Problem:** Unlike the `doctor` command which validates addresses using `auth.ExtractHostname()` (main.go:158), the `contexts add` command accepts any address value including malformed URLs like "://broken", "not-a-url", or empty strings. This allows invalid configurations to be saved, causing confusing failures later when other commands try to use the context.

**Current code:**
```go
func (c *ContextsAddCmd) Run() error {
    settings, err := config.Load(c.baseDir)
    // ...
    settings.Contexts[c.Name] = config.Context{
        Address:    c.CtxAddress,  // No validation!
        DefaultOrg: c.DefaultOrg,
        LogLevel:   c.LogLevel,
    }
    // ...
}
```

**Fix:** Add address validation before saving:
```go
func (c *ContextsAddCmd) Run() error {
    settings, err := config.Load(c.baseDir)
    if err != nil {
        return internalcmd.NewRuntimeError(err)
    }

    if _, exists := settings.Contexts[c.Name]; exists {
        return internalcmd.NewRuntimeError(fmt.Errorf("context %q already exists", c.Name))
    }

    // Validate address format
    if _, err := auth.ExtractHostname(c.CtxAddress); err != nil {
        return internalcmd.NewRuntimeError(fmt.Errorf("invalid address %q: %w", c.CtxAddress, err))
    }

    settings.Contexts[c.Name] = config.Context{
        // ...
    }
}
```

**Test to add:** `TestContextsAddCmd_InvalidAddressRejected`

**Plan (2026-01-21):**
- Acceptance criteria: `tfc contexts add bad --ctx-address "://broken"` returns RuntimeError with message containing "invalid address"
- Verification: Add test `TestContextsAddCmd_InvalidAddressRejected` that verifies error is returned and is RuntimeError type
- Implementation:
  1. Add address validation using `auth.ExtractHostname()` after checking context doesn't exist
  2. Add test case in `contexts_test.go`
  3. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/main.go:415-418` - Added address validation using `auth.ExtractHostname()` after checking context doesn't exist
- `cmd/tfc/contexts_test.go:109-147` - Added `TestContextsAddCmd_InvalidAddressRejected` test with subtests for malformed URL and empty hostname cases
- `cmd/tfc/contexts_test.go:4` - Added `strings` import for `strings.Contains`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsAddCmd_InvalidAddressRejected" ./cmd/tfc/...` - pass (both subtests pass)

---

## Edge Cases

### 49. Show Command Displays Empty Default Org

**File:** `cmd/tfc/main.go:537`

**Problem:** When `--default-org` is not set, `ContextsShowCmd` displays `Default Org:` followed by nothing, which looks incomplete and could confuse users.

**Current code:**
```go
fmt.Printf("  Default Org: %s\n", resolved.DefaultOrg)
```

**Output when no org set:**
```
Context: default (current)
  Address:     app.terraform.io
  Default Org:
  Log Level:   info
```

**Fix:** Display "(none)" when empty:
```go
defaultOrg := resolved.DefaultOrg
if defaultOrg == "" {
    defaultOrg = "(none)"
}
fmt.Fprintf(c.stdout, "  Default Org: %s\n", defaultOrg)
```

---

### 50. ContextsListCmd Signature Missing CLI Parameter

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

### 51. ContextsAddCmd Needs CLI Parameter for Consistency

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

### 52. Missing Test: ContextsListCmd Settings File Not Found

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Load()` fails (e.g., settings file doesn't exist).

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsListCmd_NoSettings` exists and verifies that when settings file doesn't exist, ContextsListCmd returns an error
- Verification: Run `go test -v -run "TestContextsListCmd_NoSettings" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestContextsListCmd_NoSettings` to `contexts_test.go`
  2. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:414-424` - Added `TestContextsListCmd_NoSettings` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsListCmd_NoSettings" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsListCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsListCmd_NoSettings(t *testing.T) {
    tmpHome := t.TempDir()
    // Don't create settings file

    cmd := &ContextsListCmd{baseDir: tmpHome}

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when settings not found, got nil")
    }
}
```

---

### 53. Missing Test: ContextsAddCmd Settings File Not Found

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Load()` fails before adding a context.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsAddCmd_NoSettings` exists and verifies that when settings file doesn't exist, ContextsAddCmd returns an error
- Verification: Run `go test -v -run "TestContextsAddCmd_NoSettings" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestContextsAddCmd_NoSettings` to `contexts_test.go`
  2. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:149-162` - Added `TestContextsAddCmd_NoSettings` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsAddCmd_NoSettings" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsAddCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsAddCmd_NoSettings(t *testing.T) {
    tmpHome := t.TempDir()

    cmd := &ContextsAddCmd{
        Name:       "new-context",
        CtxAddress: "tfe.example.com",
        baseDir:    tmpHome,
    }

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when settings not found, got nil")
    }
}
```

---

### 54. Missing Test: ContextsUseCmd Settings File Not Found

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Load()` fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsUseCmd_NoSettings` exists and verifies that when settings file doesn't exist, ContextsUseCmd returns an error
- Verification: Run `go test -v -run "TestContextsUseCmd_NoSettings" ./cmd/tfc/...`
- Implementation:
  1. Add test `TestContextsUseCmd_NoSettings` to `contexts_test.go`
  2. Run feedback loops

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:222-233` - Added `TestContextsUseCmd_NoSettings` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsUseCmd_NoSettings" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsUseCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsUseCmd_NoSettings(t *testing.T) {
    tmpHome := t.TempDir()

    cmd := &ContextsUseCmd{
        Name:    "some-context",
        baseDir: tmpHome,
    }

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when settings not found, got nil")
    }
}
```

---

### 55. Missing Test: ContextsRemoveCmd Settings File Not Found

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Load()` fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsRemoveCmd_NoSettings` exists and verifies that when settings file doesn't exist, ContextsRemoveCmd returns an error
- Verification: Run `go test -v -run "TestContextsRemoveCmd_NoSettings" ./cmd/tfc/...`
- Implementation: Add test following existing NoSettings test pattern

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:379-394` - Added `TestContextsRemoveCmd_NoSettings` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsRemoveCmd_NoSettings" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsRemoveCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsRemoveCmd_NoSettings(t *testing.T) {
    tmpHome := t.TempDir()

    forceVal := true
    cmd := &ContextsRemoveCmd{
        Name:      "some-context",
        baseDir:   tmpHome,
        forceFlag: &forceVal,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when settings not found, got nil")
    }
}
```

---

### 56. Missing Test: ContextsShowCmd Settings File Not Found

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Load()` fails.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsShowCmd_NoSettings` exists and verifies that when settings file doesn't exist, ContextsShowCmd returns an error
- Verification: Run `go test -v -run "TestContextsShowCmd_NoSettings" ./cmd/tfc/...`
- Implementation: Add test following existing NoSettings test pattern

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:478-489` - Added `TestContextsShowCmd_NoSettings` test

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestContextsShowCmd_NoSettings" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsShowCmd_NoSettings tests error when settings file doesn't exist.
func TestContextsShowCmd_NoSettings(t *testing.T) {
    tmpHome := t.TempDir()

    cmd := &ContextsShowCmd{baseDir: tmpHome}

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when settings not found, got nil")
    }
}
```

---

### 57. Missing Test: ContextsRemoveCmd Prompter Error

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `prompter.Confirm()` returns an error. The `errorPrompter` type exists in `testhelpers_test.go`.

**Plan (2026-01-21):**
- Acceptance criteria: Test `TestContextsRemoveCmd_PrompterError` exists and verifies that when prompter.Confirm() returns an error, it is wrapped and surfaced as a RuntimeError with message "failed to prompt for confirmation"
- Verification: Run `go test -v -run "TestContextsRemoveCmd_PrompterError" ./cmd/tfc/...`
- Implementation:
  1. Use the existing shared `errorPrompter` from `testhelpers_test.go`
  2. Add test `TestContextsRemoveCmd_PrompterError` to `contexts_test.go`
  3. Need to add `errors` import to contexts_test.go for `errors.New`
  4. Run feedback loops to verify

**Progress notes (2026-01-21):**

Changes made:
- `cmd/tfc/contexts_test.go:4` - Added `errors` import
- `cmd/tfc/contexts_test.go:492-519` - Added `TestContextsRemoveCmd_PrompterError` test using shared `errorPrompter`

Verification:
- `make fmt` - passed
- `make lint` - passed (with temp caches due to permission issues)
- `make build` - passed (with temp caches)
- `make test` - all tests pass
- `go test -v -run "TestContextsRemoveCmd_PrompterError" ./cmd/tfc/...` - pass

**Test to add:**
```go
// TestContextsRemoveCmd_PrompterError tests that prompter errors are surfaced.
func TestContextsRemoveCmd_PrompterError(t *testing.T) {
    tmpHome := t.TempDir()

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
            "prod":    {Address: "tfe.example.com", LogLevel: "warn"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    cmd := &ContextsRemoveCmd{
        Name:     "prod",
        baseDir:  tmpHome,
        prompter: &errorPrompter{err: errors.New("terminal not available")},
    }
    cli := &CLI{Force: false}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
        t.Errorf("expected prompt error, got: %v", err)
    }
}
```

---

### 58. Missing Test: ContextsAddCmd config.Save Failure

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies error handling when `config.Save()` fails (e.g., read-only filesystem).

**Test to add:**
```go
// TestContextsAddCmd_SaveError tests that save errors are properly surfaced.
func TestContextsAddCmd_SaveError(t *testing.T) {
    tmpHome := t.TempDir()

    // Create initial settings
    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    // Make directory read-only to prevent writing
    tfccliDir := filepath.Join(tmpHome, ".tfccli")
    if err := os.Chmod(tfccliDir, 0o500); err != nil {
        t.Fatalf("Failed to chmod: %v", err)
    }
    t.Cleanup(func() {
        os.Chmod(tfccliDir, 0o700) // Restore so cleanup can delete
    })

    cmd := &ContextsAddCmd{
        Name:       "new-context",
        CtxAddress: "tfe.example.com",
        baseDir:    tmpHome,
    }

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when save fails, got nil")
    }
    if !strings.Contains(err.Error(), "failed to save settings") {
        t.Errorf("expected save failure message, got: %v", err)
    }
}
```

---

### 59. Missing Test: ContextsUseCmd config.Save Failure

**File:** `cmd/tfc/contexts_test.go`

**Problem:** Same as #58 but for the use command.

**Test to add:**
```go
// TestContextsUseCmd_SaveError tests that save errors are properly surfaced.
func TestContextsUseCmd_SaveError(t *testing.T) {
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

    cmd := &ContextsUseCmd{
        Name:    "prod",
        baseDir: tmpHome,
    }

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error when save fails, got nil")
    }
    if !strings.Contains(err.Error(), "failed to save settings") {
        t.Errorf("expected save failure message, got: %v", err)
    }
}
```

---

### 60. Missing Test: ContextsRemoveCmd config.Save Failure

**File:** `cmd/tfc/contexts_test.go`

**Problem:** Same as #58 but for the remove command.

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

### 61. Missing Test: ContextsAddCmd Invalid Address

**File:** `cmd/tfc/contexts_test.go`

**Problem:** After implementing fix for #48, need a test to verify invalid addresses are rejected.

**Test to add (after #48 is fixed):**
```go
// TestContextsAddCmd_InvalidAddressRejected tests that malformed addresses are rejected.
func TestContextsAddCmd_InvalidAddressRejected(t *testing.T) {
    tmpHome := t.TempDir()

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    cmd := &ContextsAddCmd{
        Name:       "bad-context",
        CtxAddress: "://invalid-url",
        baseDir:    tmpHome,
    }

    err := cmd.Run()
    if err == nil {
        t.Fatal("expected error for invalid address, got nil")
    }
    if !strings.Contains(err.Error(), "invalid address") {
        t.Errorf("expected invalid address error, got: %v", err)
    }
}
```

---

### 62. Missing Test: ContextsRemoveCmd Context Not Found

**File:** `cmd/tfc/contexts_test.go`

**Problem:** No test verifies the error when trying to remove a nonexistent context.

**Test to add:**
```go
// TestContextsRemoveCmd_ContextNotFound tests error when removing nonexistent context.
func TestContextsRemoveCmd_ContextNotFound(t *testing.T) {
    tmpHome := t.TempDir()

    settings := &config.Settings{
        CurrentContext: "default",
        Contexts: map[string]config.Context{
            "default": {Address: "app.terraform.io", LogLevel: "info"},
        },
    }
    createTestSettings(t, tmpHome, settings)

    forceVal := true
    cmd := &ContextsRemoveCmd{
        Name:      "nonexistent",
        baseDir:   tmpHome,
        forceFlag: &forceVal,
    }
    cli := &CLI{}

    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when context not found, got nil")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected 'not found' error, got: %v", err)
    }
}
```

---

### 63. Tests Don't Verify Output Content

**File:** `cmd/tfc/contexts_test.go:18-38, 306-326`

**Problem:** Several tests pass but include comments noting they don't verify stdout content. After fixing #46 (injectable stdout), these tests should be updated to capture and verify output.

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

### 64. Missing Test: ContextsListCmd JSON Output

**File:** `cmd/tfc/contexts_test.go`

**Problem:** After implementing JSON output support (#45), needs tests for JSON format.

**Test to add (after #45 is fixed):**
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

### 65. Missing Test: ContextsShowCmd JSON Output

**File:** `cmd/tfc/contexts_test.go`

**Problem:** After implementing JSON output support, needs test for JSON format in show command.

**Test to add (after #45 is fixed):**
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

## Contexts Command Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| List all contexts | ✅ | - |
| List JSON output | ❌ | #64 |
| List output verification | ❌ | #63 |
| List settings not found | ✅ | #52 |
| Add new context | ✅ | - |
| Add existing context (error) | ✅ | - |
| Add with invalid address | ✅ | #48 |
| Add settings not found | ✅ | #53 |
| Add config.Save failure | ❌ | #58 |
| Use/switch context | ✅ | - |
| Use nonexistent context (error) | ✅ | - |
| Use settings not found | ✅ | #54 |
| Use config.Save failure | ❌ | #59 |
| Remove with --force | ✅ | - |
| Remove current context (error) | ✅ | - |
| Remove with prompt (decline) | ✅ | - |
| Remove with prompt (accept) | ✅ | - |
| Remove nonexistent context | ❌ | #62 |
| Remove settings not found | ✅ | #55 |
| Remove config.Save failure | ❌ | #60 |
| Remove prompter error | ✅ | #57 |
| Show current context | ✅ | - |
| Show named context | ✅ | - |
| Show nonexistent context (error) | ✅ | - |
| Show JSON output | ❌ | #65 |
| Show output verification | ❌ | #63 |
| Show settings not found | ✅ | #56 |

---

# Workspace-Resources Subcommand Code Review

Review of `cmd/tfc/workspace_resources.go` and `cmd/tfc/workspace_resources_test.go`.

---

## Bugs

### 66. Table Column Label and Data Mismatch

**Status: DONE** (2026-01-21)

**File:** `cmd/tfc/workspace_resources.go:176-179`

**Problem:** The table headers and data columns are misaligned. The header "PROVIDER-TYPE" (4th column) receives `r.Provider` (the provider name like "aws"), while the header "TYPE" (2nd column) receives `r.ProviderType` (the resource type like "aws_instance"). The labels are semantically swapped.

**Current code:**
```go
tw := output.NewTableWriter(c.stdout, []string{"ID", "TYPE", "NAME", "PROVIDER-TYPE"}, isTTY)
for _, r := range resources {
    // Provider type is the resource type (e.g., "aws_instance")
    tw.AddRow(r.ID, r.ProviderType, r.Name, r.Provider)
}
```

**Actual output:**
```
ID      TYPE           NAME   PROVIDER-TYPE
res-1   aws_instance   web    aws
```

**Expected behavior:** Either:
1. Fix the headers to match the data: `["ID", "RESOURCE-TYPE", "NAME", "PROVIDER"]`
2. Or fix the data order to match the headers (swap columns 2 and 4)

**Fix (Option 1 - recommended, clearer headers):**
```go
tw := output.NewTableWriter(c.stdout, []string{"ID", "RESOURCE-TYPE", "NAME", "PROVIDER"}, isTTY)
for _, r := range resources {
    tw.AddRow(r.ID, r.ProviderType, r.Name, r.Provider)
}
```

**Progress notes (2026-01-21):**

Plan:
- Acceptance criteria: Table headers accurately describe the data in each column:
  - Column 1: ID → r.ID
  - Column 2: RESOURCE-TYPE → r.ProviderType (like "aws_instance")
  - Column 3: NAME → r.Name
  - Column 4: PROVIDER → r.Provider (like "aws")
- Verification: Run existing table output test `TestWorkspaceResourcesList_Table`
- Implementation:
  1. Change header from `["ID", "TYPE", "NAME", "PROVIDER-TYPE"]` to `["ID", "RESOURCE-TYPE", "NAME", "PROVIDER"]`
  2. Remove misleading comment on line 178
  3. Update test to verify headers match

Changes made:
- `cmd/tfc/workspace_resources.go:176` - Changed headers to `["ID", "RESOURCE-TYPE", "NAME", "PROVIDER"]`
- `cmd/tfc/workspace_resources.go:178` - Removed misleading comment about provider type
- `cmd/tfc/workspace_resources_test.go:172-173` - Updated test to verify new headers

Verification:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run "TestWorkspaceResourcesList_Table" ./cmd/tfc/...` - pass

---

## Missing Features

### 67. Missing Get Subcommand

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

### 68. Missing Test: Client Factory Error

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error. This tests lines 148-151 in the Run method.

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

### 69. Missing Test: Context Not Found

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when the specified `--context` flag references a non-existent context.

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

### 70. Missing Test: Token Resolution Error

**File:** `cmd/tfc/workspace_resources_test.go`

**Problem:** No test verifies error handling when token resolution fails (e.g., no token available for the address).

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

---

### 71. Missing Test: Table Output Column Verification

**File:** `cmd/tfc/workspace_resources_test.go:144-180`

**Problem:** `TestWorkspaceResourcesList_Table` only verifies that headers and resource ID/name appear in output, but doesn't verify the actual column order or that the correct data appears in the correct columns. After fixing #66, this test should verify the fix.

**Enhanced test:**
```go
// TestWorkspaceResourcesList_Table_ColumnVerification tests that table columns match headers.
func TestWorkspaceResourcesList_Table_ColumnVerification(t *testing.T) {
    tmpDir, resolver := setupWorkspaceResourcesTestSettings(t)

    fakeClient := &fakeWorkspaceResourcesClient{
        resources: []*tfe.WorkspaceResource{
            {ID: "res-1", Address: "aws_instance.web", Name: "web", ProviderType: "aws_instance", Provider: "aws"},
        },
    }

    var buf bytes.Buffer
    cmd := &WorkspaceResourcesListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &buf,
        clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := buf.String()
    lines := strings.Split(strings.TrimSpace(out), "\n")
    if len(lines) < 2 {
        t.Fatalf("expected at least 2 lines (header + data), got %d", len(lines))
    }

    // After fix #66, verify headers are sensible
    // Header line should contain column names in order
    if !strings.Contains(lines[0], "ID") {
        t.Error("expected ID column in header")
    }

    // Data line should have resource values in expected positions
    dataLine := lines[1]
    if !strings.Contains(dataLine, "res-1") {
        t.Error("expected resource ID in data line")
    }
    if !strings.Contains(dataLine, "aws_instance") {
        t.Error("expected provider type (aws_instance) in data line")
    }
    if !strings.Contains(dataLine, "web") {
        t.Error("expected resource name in data line")
    }
}
```

---

## Code Quality Improvements

### 72. Duplicate `resolveWorkspaceResourcesClientConfig` Function

**File:** `cmd/tfc/workspace_resources.go:84-117`

**Problem:** `resolveWorkspaceResourcesClientConfig` is nearly identical to `resolveVariablesClientConfig` in `workspace_variables.go` and similar functions in other command files. The only difference is that workspace-resources and workspace-variables don't return an org (since workspace ID is passed directly).

**Current code:** 33 lines of duplicate config resolution logic.

**Fix:** Use the shared helper pattern suggested in finding #24. After creating a shared `resolveClientConfig` helper, update workspace_resources.go:

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

### 73. Duplicate Test Helper Types

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

**Fix:** Move these types to `cmd/tfc/testhelpers_test.go` as `testEnv` and `testFS`, then update all test files to use the shared types. This aligns with findings #13, #25, and #42 which identified the same duplication pattern.

---

### 74. Inline TTY Detection Pattern

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

**Fix:** After moving `resolveFormat` from `projects.go` to a shared location (as suggested in finding #14), use the helper:

```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

## Workspace-Resources Test Coverage Summary

| Scenario | Currently Covered | Finding # |
|----------|-------------------|-----------|
| List JSON output | ✅ | - |
| List table output | ✅ | - |
| List API error | ✅ | - |
| List missing settings | ✅ | - |
| List empty resources | ✅ | - |
| List with --context override | ✅ | - |
| List with --address override | ✅ | - |
| List client factory error | ❌ | #68 |
| List context not found | ❌ | #69 |
| List token resolution error | ❌ | #70 |
| List table column verification | ❌ | #71 |
| Get by ID | ❌ | #67 |
| Get not found (404) | ❌ | #67 |
| Get generic API error | ❌ | #67 |
