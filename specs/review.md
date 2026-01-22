# Code Review: Runs Subcommand

## Files Reviewed
- `cmd/tfc/runs.go` - Main implementation (732 lines)
- `cmd/tfc/runs_test.go` - Unit tests (893 lines)
- `internal/tfcapi/pagination.go` - CollectAllRuns function
- `cmd/tfc/common.go` - Shared helpers (resolveFormat, resolveClientConfig)

---

## Issues Found

### 1. [x] Runs commands don't use `resolveFormat` helper

**Status:** DONE

**File:** `cmd/tfc/runs.go`
**Lines:** 196-200, 265-269, 351-355, 443-447, 534-538, 625-629, 715-719

**Problem:** All runs commands duplicate TTY detection logic inline instead of using the `resolveFormat` helper from `common.go`. Other commands like `doctor`, `projects`, `workspace-variables`, and `workspace-resources` use this helper consistently.

**Current code (repeated 7 times):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Replace with:
```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

#### Plan
- Acceptance criteria: All 7 runs commands use `resolveFormat` helper instead of inline TTY detection.
- Verification: `make fmt && make lint && make test` passes; no functional change in behavior.
- Implementation steps:
  1. Replace inline TTY detection in RunsListCmd.Run (lines 196-200)
  2. Replace inline TTY detection in RunsGetCmd.Run (lines 265-269)
  3. Replace inline TTY detection in RunsCreateCmd.Run (lines 351-355)
  4. Replace inline TTY detection in RunsApplyCmd.Run (lines 443-447)
  5. Replace inline TTY detection in RunsDiscardCmd.Run (lines 534-538)
  6. Replace inline TTY detection in RunsCancelCmd.Run (lines 625-629)
  7. Replace inline TTY detection in RunsForceCancelCmd.Run (lines 715-719)

#### Progress Notes

**2026-01-22:** Completed refactor.
- Changed: `cmd/tfc/runs.go` - replaced 7 instances of inline TTY detection with `resolveFormat` helper
- Commands: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Result: Code is now consistent with other commands (doctor, projects, workspace-variables, workspace-resources)

---

### 2. [x] `runJSON` struct missing workspace_id field

**Status:** DONE

**File:** `cmd/tfc/runs.go`
**Lines:** 32-38, 41-48

**Problem:** The `runJSON` struct doesn't include workspace ID, but `RunsGetCmd` table output shows it (lines 283-285). This creates inconsistent output between JSON and table formats.

**Fix:** Add workspace_id to the struct:
```go
type runJSON struct {
    ID          string `json:"id"`
    Status      string `json:"status"`
    Message     string `json:"message,omitempty"`
    CreatedAt   string `json:"created_at"`
    Source      string `json:"source,omitempty"`
    WorkspaceID string `json:"workspace_id,omitempty"` // Add this
}
```

And update `toRunJSON`:
```go
func toRunJSON(run *tfe.Run) *runJSON {
    r := &runJSON{
        ID:        run.ID,
        Status:    string(run.Status),
        Message:   run.Message,
        CreatedAt: run.CreatedAt.Format(time.RFC3339),
        Source:    string(run.Source),
    }
    if run.Workspace != nil {
        r.WorkspaceID = run.Workspace.ID
    }
    return r
}
```

#### Plan
- **Acceptance criteria:** JSON output includes `workspace_id` field when the run has a workspace, matching table output behavior.
- **Verification:** Tests pass; JSON output contains workspace_id when run.Workspace is not nil; field is omitted (omitempty) when nil.
- **Implementation steps:**
  1. Add `WorkspaceID string` field to `runJSON` struct with `json:"workspace_id,omitempty"` tag
  2. Update `toRunJSON` function to conditionally populate `WorkspaceID` from `run.Workspace.ID`
  3. Update existing tests to include workspace in test runs and verify JSON output
  4. Run `make fmt && make lint && make test` to verify

#### Progress Notes

**2026-01-22:** Completed.
- Changed: `cmd/tfc/runs.go` - added `WorkspaceID` field to `runJSON` struct with `json:"workspace_id,omitempty"` tag; updated `toRunJSON` to conditionally populate it from `run.Workspace.ID`
- Changed: `cmd/tfc/runs_test.go` - added `TestRunsGet_JSON_WithWorkspace` test to verify workspace_id appears in JSON output
- Commands: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Result: JSON and table output are now consistent - both include workspace_id when workspace is present

---

### 3. [x] `fakeRunsClient.forceCancel` field naming inconsistency

**Status:** DONE

**File:** `cmd/tfc/runs_test.go`
**Line:** 32

**Problem:** The error field is named `forceCancel` but all other error fields follow the pattern `<action>Err` (e.g., `applyErr`, `discardErr`, `cancelErr`).

**Current:**
```go
forceCancel       error
```

**Fix:** Rename to match convention:
```go
forceCancelErr    error
```

Also update line 79:
```go
return c.forceCancelErr
```

#### Plan
- **Acceptance criteria:** Field renamed from `forceCancel` to `forceCancelErr` to match convention of other error fields.
- **Verification:** `make fmt && make lint && make test` passes; no functional change.
- **Implementation steps:**
  1. Rename `forceCancel` field to `forceCancelErr` on line 32
  2. Update reference on line 79 to use `forceCancelErr`

#### Progress Notes

**2026-01-22:** Completed.
- Changed: `cmd/tfc/runs_test.go` - renamed `forceCancel` field to `forceCancelErr` (line 32) and updated reference (line 79)
- Commands: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Result: Field naming is now consistent with `applyErr`, `discardErr`, `cancelErr`

---

### 4. [ ] `fakeRunsClient` doesn't capture parameters for verification

**File:** `cmd/tfc/runs_test.go`
**Lines:** 40-79

**Problem:** The fake client ignores most parameters (workspaceID, runID, options), making it impossible to verify that commands pass correct values to the API. Only `createOpts` is captured.

**Fix:** Add fields to capture parameters:
```go
type fakeRunsClient struct {
    // ... existing fields ...

    // Captured parameters for verification
    listWorkspaceID   string
    listOpts          *tfe.RunListOptions
    readRunID         string
    applyRunID        string
    applyOpts         tfe.RunApplyOptions
    discardRunID      string
    discardOpts       tfe.RunDiscardOptions
    cancelRunID       string
    cancelOpts        tfe.RunCancelOptions
    forceCancelRunID  string
    forceCancelOpts   tfe.RunForceCancelOptions
}
```

Then update the methods to capture these values.

---

### 5. [ ] Missing test: `RunsGet` table output

**File:** `cmd/tfc/runs_test.go`

**Problem:** There's `TestRunsGet_JSON` but no test for table output format, which has different logic (field/value pairs instead of data object).

**Fix:** Add test:
```go
func TestRunsGet_Table(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
    fakeClient := &fakeRunsClient{
        run: &tfe.Run{
            ID:        "run-1",
            Status:    tfe.RunPlanned,
            Message:   "Test run",
            CreatedAt: createdAt,
            Source:    tfe.RunSourceAPI,
            Workspace: &tfe.Workspace{ID: "ws-test"},
        },
    }

    var stdout bytes.Buffer
    cmd := &RunsGetCmd{
        ID:            "run-1",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := stdout.String()
    if !strings.Contains(out, "run-1") {
        t.Errorf("expected run ID in output, got: %s", out)
    }
    if !strings.Contains(out, "Workspace ID") {
        t.Errorf("expected Workspace ID field, got: %s", out)
    }
}
```

---

### 6. [ ] Missing test: empty runs list

**File:** `cmd/tfc/runs_test.go`

**Problem:** No test verifies behavior when workspace has zero runs. Both JSON and table output should handle empty results gracefully.

**Fix:** Add test:
```go
func TestRunsList_EmptyList(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    fakeClient := &fakeRunsClient{
        runs: []*tfe.Run{}, // Empty list
    }

    var stdout bytes.Buffer
    cmd := &RunsListCmd{
        WorkspaceID:   "ws-empty",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var result map[string]any
    if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    data, ok := result["data"].([]any)
    if !ok {
        t.Fatalf("expected data array, got %T", result["data"])
    }
    if len(data) != 0 {
        t.Errorf("expected 0 runs, got %d", len(data))
    }
}
```

---

### 7. [ ] Missing test: `RunsCreate` API error

**File:** `cmd/tfc/runs_test.go`

**Problem:** While `TestRunsList_APIError` tests list failure, there's no equivalent for `RunsCreate`.

**Fix:** Add test:
```go
func TestRunsCreate_APIError(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    fakeClient := &fakeRunsClient{
        createErr: errors.New("workspace not found"),
    }

    var stdout bytes.Buffer
    cmd := &RunsCreateCmd{
        WorkspaceID:   "ws-invalid",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for API failure")
    }
    if !strings.Contains(err.Error(), "workspace not found") {
        t.Errorf("expected error message, got: %v", err)
    }
}
```

---

### 8. [ ] Missing test: Comment option passed to Apply/Discard/Cancel/ForceCancel

**File:** `cmd/tfc/runs_test.go`

**Problem:** The `--comment` flag functionality is not tested. We don't verify the comment is actually passed to the API.

**Fix:** Add a test (example for Apply, repeat pattern for others):
```go
func TestRunsApply_WithComment(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    fakeClient := &fakeRunsClient{}

    var stdout bytes.Buffer
    forceFlag := true
    cmd := &RunsApplyCmd{
        ID:            "run-1",
        Comment:       "LGTM, applying",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return fakeClient, nil
        },
        forceFlag: &forceFlag,
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Requires fakeRunsClient to capture applyOpts (see issue #4)
    if fakeClient.applyOpts.Comment == nil || *fakeClient.applyOpts.Comment != "LGTM, applying" {
        t.Error("expected comment to be passed to API")
    }
}
```

---

### 9. [ ] Missing test: client factory returns error

**File:** `cmd/tfc/runs_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error.

**Fix:** Add test:
```go
func TestRunsList_ClientFactoryError(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    var stdout bytes.Buffer
    cmd := &RunsListCmd{
        WorkspaceID:   "ws-test",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return nil, errors.New("failed to create TFC client")
        },
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for client factory failure")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client error message, got: %v", err)
    }
}
```

---

### 10. [ ] Missing test: invalid context specified via --context flag

**File:** `cmd/tfc/runs_test.go`

**Problem:** `TestRunsList_FailsWhenSettingsMissing` tests missing settings file, but no test for when `--context` flag specifies a non-existent context.

**Fix:** Add test:
```go
func TestRunsList_InvalidContext(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    var stdout bytes.Buffer
    cmd := &RunsListCmd{
        WorkspaceID:   "ws-test",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return &fakeRunsClient{}, nil
        },
    }

    cli := &CLI{Context: "nonexistent"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid context")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected context not found error, got: %v", err)
    }
}
```

---

### 11. [ ] Missing test: prompter error handling

**File:** `cmd/tfc/runs_test.go`

**Problem:** While `runsFailingPrompter` verifies prompts are bypassed with `--force`, there's no test that verifies error handling when `prompter.Confirm` fails and returns an error during normal (non-force) flow.

**Fix:** Add a prompter that returns an error:
```go
type runsErrorPrompter struct{}

func (p *runsErrorPrompter) PromptString(_, _ string) (string, error) {
    return "", errors.New("stdin closed")
}

func (p *runsErrorPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, errors.New("stdin closed")
}

func (p *runsErrorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", errors.New("stdin closed")
}

func TestRunsApply_PrompterError(t *testing.T) {
    tmpDir, resolver := setupRunsTest(t)

    fakeClient := &fakeRunsClient{}

    var stdout bytes.Buffer
    cmd := &RunsApplyCmd{
        ID:            "run-1",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
            return fakeClient, nil
        },
        prompter: &runsErrorPrompter{},
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for prompter failure")
    }
    if !strings.Contains(err.Error(), "failed to prompt") {
        t.Errorf("expected prompt error, got: %v", err)
    }
    if fakeClient.applyCalled {
        t.Error("apply should not be called when prompt fails")
    }
}
```

---

### 12. [ ] Inconsistent success message for Apply command

**File:** `cmd/tfc/runs.go`
**Line:** 454

**Problem:** Apply says "apply initiated" while other actions use past tense ("discarded", "cancelled", "force-cancelled"). This inconsistency could confuse users.

**Current:**
- Apply: `Run %q apply initiated.`
- Discard: `Run %q discarded.`
- Cancel: `Run %q cancelled.`
- ForceCancel: `Run %q force-cancelled.`

**Fix:** Either change Apply to `Run %q applied.` or change others to match "initiated" style if that's intentional (Apply is async). Document the reason if intentional.

---

### 13. [ ] `RunsListCmd` lacks `--limit` flag for large workspaces

**File:** `cmd/tfc/runs.go`
**Lines:** 151-161, 185-186

**Problem:** `RunsListCmd` always fetches ALL runs via `CollectAllRuns`. For workspaces with thousands of runs, this is slow and memory-intensive. Users can't limit results.

**Fix:** Add a `--limit` flag:
```go
type RunsListCmd struct {
    WorkspaceID string `name:"workspace-id" required:"" help:"ID of the workspace."`
    Limit       int    `help:"Maximum number of runs to return (0 = all)." default:"0"`
    // ... other fields
}
```

Then implement pagination limit in the Run method, or add a separate `CollectRunsWithLimit` function.

---

### 14. [ ] Test assertion uses fragile type assertion

**File:** `cmd/tfc/runs_test.go`
**Lines:** 469-471

**Problem:** The test uses `meta["status"].(float64)` which will panic if the type changes or is missing, rather than failing gracefully.

**Current:**
```go
meta := result["meta"].(map[string]any)
if meta["status"].(float64) != 202 {
```

**Fix:** Use safer pattern:
```go
meta, ok := result["meta"].(map[string]any)
if !ok {
    t.Fatal("expected meta object in response")
}
status, ok := meta["status"].(float64)
if !ok {
    t.Fatalf("expected status to be number, got %T", meta["status"])
}
if status != 202 {
    t.Errorf("expected status 202, got %v", status)
}
```

---

### 15. [ ] Missing test: verify correct run ID passed to Read/Apply/Discard/Cancel/ForceCancel

**File:** `cmd/tfc/runs_test.go`

**Problem:** Tests verify the API methods are called, but don't verify the correct run ID is passed. If a bug caused the wrong ID to be used, tests would still pass.

**Fix:** After fixing issue #4 (capture parameters), add assertions:
```go
// In TestRunsApply_WithForce
if fakeClient.applyRunID != "run-1" {
    t.Errorf("expected run ID run-1, got %s", fakeClient.applyRunID)
}
```

---

### 16. [ ] Missing test: verify correct workspace ID passed to List

**File:** `cmd/tfc/runs_test.go`

**Problem:** Same as #15, but for the List command. The test doesn't verify that the workspace ID is correctly passed to the API.

**Fix:** After fixing issue #4, add assertion:
```go
// In TestRunsList_JSON
if fakeClient.listWorkspaceID != "ws-test" {
    t.Errorf("expected workspace ID ws-test, got %s", fakeClient.listWorkspaceID)
}
```

---

# Code Review: Plans Subcommand

## Files Reviewed
- `cmd/tfc/plans.go` - Main implementation (390 lines)
- `cmd/tfc/plans_test.go` - Unit tests (710 lines)
- `cmd/tfc/common.go` - Shared helpers (resolveFormat, resolveClientConfig)

---

## Issues Found

### 17. [ ] Plans commands don't use `resolveFormat` helper

**File:** `cmd/tfc/plans.go`
**Lines:** 163-167, 237-241, 339-343

**Problem:** All three plans commands (`PlansGetCmd`, `PlansJSONOutputCmd`, `PlansSanitizedPlanCmd`) duplicate TTY detection logic inline instead of using the `resolveFormat` helper from `common.go`. Other commands like `doctor`, `projects`, `workspace-variables`, and `workspace-resources` use this helper consistently.

**Current code (repeated 3 times):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Replace with:
```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

### 18. [ ] `resolvePlansClientConfig` duplicates `resolveClientConfig` from common.go

**File:** `cmd/tfc/plans.go`
**Lines:** 84-116

**Problem:** The `resolvePlansClientConfig` function is nearly identical to `resolveClientConfig` in `common.go`, minus the organization resolution. This duplicates ~30 lines of code and creates maintenance burden.

**Current:** Two separate functions with identical context/settings/token resolution logic.

**Fix:** Either:
1. Refactor `resolveClientConfig` to make org optional (return empty string if not needed)
2. Create a shared helper that both functions use
3. Use `resolveClientConfig` and ignore the org return value:
```go
cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
```

---

### 19. [ ] `planJSON` missing `LogReadURL` in table output

**File:** `cmd/tfc/plans.go`
**Lines:** 27-36, 175-185

**Problem:** The `planJSON` struct includes `LogReadURL` field (line 35), and JSON output includes it. However, table output (lines 175-185) doesn't display LogReadURL, creating inconsistency between formats.

**Fix:** Add LogReadURL to table output:
```go
tw.AddRow("Imports", fmt.Sprintf("%d", plan.ResourceImports))
if plan.LogReadURL != "" {
    tw.AddRow("Log URL", plan.LogReadURL)
}
```

---

### 20. [ ] `defaultDownloadClient` doesn't include response body in error

**File:** `cmd/tfc/plans.go`
**Lines:** 377-389

**Problem:** When the HTTP status code is not 200, the function returns a generic error with just the status code. The response body (which often contains useful error details from the server) is discarded.

**Current:**
```go
if resp.StatusCode != http.StatusOK {
    return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
```

**Fix:** Read and include the response body in the error:
```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    if len(body) > 0 {
        return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
    }
    return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
```

---

### 21. [ ] Misleading error message when `sanitized-plan` link has wrong type

**File:** `cmd/tfc/plans.go`
**Lines:** 327-329

**Problem:** The type assertion `plan.Links["sanitized-plan"].(string)` returns `ok=false` both when the key is missing AND when the value is not a string. The error message "sanitized plan not available" is misleading if the link exists but has the wrong type (e.g., an integer or object).

**Current:**
```go
sanitizedPlanLink, ok := plan.Links["sanitized-plan"].(string)
if !ok || sanitizedPlanLink == "" {
    return internalcmd.NewRuntimeError(fmt.Errorf("sanitized plan not available for this plan (HYOK feature)"))
}
```

**Fix:** Check for existence separately from type:
```go
linkVal, exists := plan.Links["sanitized-plan"]
if !exists {
    return internalcmd.NewRuntimeError(fmt.Errorf("sanitized plan not available for this plan (HYOK feature)"))
}
sanitizedPlanLink, ok := linkVal.(string)
if !ok || sanitizedPlanLink == "" {
    return internalcmd.NewRuntimeError(fmt.Errorf("sanitized plan link has unexpected type: %T", linkVal))
}
```

---

### 22. [ ] `fakePlansClient` doesn't capture `planID` for verification

**File:** `cmd/tfc/plans_test.go`
**Lines:** 22-41

**Problem:** The fake client ignores the `planID` parameter in `Read` and `ReadJSONOutput`, making it impossible to verify that commands pass the correct plan ID to the API.

**Current:**
```go
func (f *fakePlansClient) Read(_ context.Context, _ string) (*tfe.Plan, error) {
```

**Fix:** Add fields to capture parameters:
```go
type fakePlansClient struct {
    plan       *tfe.Plan
    jsonOutput []byte
    readErr    error
    jsonErr    error

    // Captured parameters for verification
    readPlanID       string
    jsonOutputPlanID string
}

func (f *fakePlansClient) Read(_ context.Context, planID string) (*tfe.Plan, error) {
    f.readPlanID = planID
    if f.readErr != nil {
        return nil, f.readErr
    }
    return f.plan, nil
}

func (f *fakePlansClient) ReadJSONOutput(_ context.Context, planID string) ([]byte, error) {
    f.jsonOutputPlanID = planID
    if f.jsonErr != nil {
        return nil, f.jsonErr
    }
    return f.jsonOutput, nil
}
```

Then add assertions in tests:
```go
// In TestPlansGet_JSON
if fakeClient.readPlanID != "plan-123" {
    t.Errorf("expected plan ID plan-123, got %s", fakeClient.readPlanID)
}
```

---

### 23. [ ] Missing test: client factory returns error

**File:** `cmd/tfc/plans_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error for any of the three commands (`PlansGetCmd`, `PlansJSONOutputCmd`, `PlansSanitizedPlanCmd`).

**Fix:** Add test:
```go
func TestPlansGet_ClientFactoryError(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    var stdout bytes.Buffer
    cmd := &PlansGetCmd{
        ID:            "plan-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return nil, errors.New("failed to create TFC client")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for client factory failure")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client error message, got: %v", err)
    }
}
```

---

### 24. [ ] Missing test: invalid context specified via --context flag

**File:** `cmd/tfc/plans_test.go`

**Problem:** `TestPlansGet_FailsWhenSettingsMissing` tests missing settings file, but no test for when `--context` flag specifies a non-existent context.

**Fix:** Add test:
```go
func TestPlansGet_InvalidContext(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    var stdout bytes.Buffer
    cmd := &PlansGetCmd{
        ID:            "plan-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return &fakePlansClient{}, nil
        },
    }

    cli := &CLI{Context: "nonexistent", OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid context")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected context not found error, got: %v", err)
    }
}
```

---

### 25. [ ] Missing test: plan with `LogReadURL` field populated

**File:** `cmd/tfc/plans_test.go`

**Problem:** No test verifies that `LogReadURL` is correctly included in JSON output when present. The existing tests don't set this field.

**Fix:** Add test:
```go
func TestPlansGet_WithLogReadURL(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    fakeClient := &fakePlansClient{
        plan: &tfe.Plan{
            ID:         "plan-123",
            Status:     tfe.PlanFinished,
            HasChanges: true,
            LogReadURL: "https://archivist.example/logs/plan-123",
        },
    }

    var stdout bytes.Buffer
    cmd := &PlansGetCmd{
        ID:            "plan-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var result map[string]any
    if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    data := result["data"].(map[string]any)
    if data["log_read_url"] != "https://archivist.example/logs/plan-123" {
        t.Errorf("expected log_read_url in output, got: %v", data["log_read_url"])
    }
}
```

---

### 26. [ ] Missing test: file write error (permission denied, directory doesn't exist)

**File:** `cmd/tfc/plans_test.go`

**Problem:** No test verifies error handling when `os.WriteFile` fails (e.g., writing to a non-existent directory or read-only location).

**Fix:** Add test:
```go
func TestPlansJSONOutput_FileWriteError(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    fakeClient := &fakePlansClient{
        jsonOutput: []byte(`{"test":"data"}`),
    }

    var stdout bytes.Buffer
    cmd := &PlansJSONOutputCmd{
        ID:            "plan-123",
        Out:           "/nonexistent/directory/out.json", // Invalid path
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid file path")
    }
    if !strings.Contains(err.Error(), "failed to write file") {
        t.Errorf("expected file write error, got: %v", err)
    }
}
```

---

### 27. [ ] Missing test: `sanitized-plan` link is non-string type

**File:** `cmd/tfc/plans_test.go`

**Problem:** `TestPlansSanitizedPlan_NoLinkAvailable` tests when the key is missing (empty map), but no test verifies behavior when the `sanitized-plan` key exists but has an unexpected type (e.g., integer, object).

**Fix:** Add test:
```go
func TestPlansSanitizedPlan_LinkWrongType(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    fakeClient := &fakePlansClient{
        plan: &tfe.Plan{
            ID: "plan-bad-link",
            Links: map[string]interface{}{
                "sanitized-plan": 12345, // Wrong type (int instead of string)
            },
        },
    }

    var stdout bytes.Buffer
    cmd := &PlansSanitizedPlanCmd{
        ID:            "plan-bad-link",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when sanitized plan link has wrong type")
    }
    // After fixing issue #21, this should mention the type
    if !strings.Contains(err.Error(), "sanitized plan") {
        t.Errorf("expected sanitized plan error, got: %v", err)
    }
}
```

---

### 28. [ ] Missing test: `plan.Links` is nil

**File:** `cmd/tfc/plans_test.go`

**Problem:** `TestPlansSanitizedPlan_NoLinkAvailable` tests with an empty map `Links: map[string]interface{}{}`, but not when `Links` is nil. Accessing a nil map in Go returns the zero value and doesn't panic, but testing this edge case ensures consistent behavior.

**Fix:** Add test:
```go
func TestPlansSanitizedPlan_NilLinks(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    fakeClient := &fakePlansClient{
        plan: &tfe.Plan{
            ID:    "plan-nil-links",
            Links: nil, // nil instead of empty map
        },
    }

    var stdout bytes.Buffer
    cmd := &PlansSanitizedPlanCmd{
        ID:            "plan-nil-links",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error when Links is nil")
    }
    if !strings.Contains(err.Error(), "sanitized plan not available") {
        t.Errorf("expected sanitized plan not available error, got: %v", err)
    }
}
```

---

### 29. [ ] Missing test: empty JSON output from `ReadJSONOutput`

**File:** `cmd/tfc/plans_test.go`

**Problem:** No test verifies behavior when `ReadJSONOutput` returns an empty byte slice. This could happen if the plan has no JSON output yet.

**Fix:** Add test:
```go
func TestPlansJSONOutput_EmptyOutput(t *testing.T) {
    tmpDir, resolver := setupPlansTest(t)

    fakeClient := &fakePlansClient{
        jsonOutput: []byte{}, // Empty
    }

    var stdout bytes.Buffer
    cmd := &PlansJSONOutputCmd{
        ID:            "plan-empty",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if stdout.Len() != 0 {
        t.Errorf("expected empty output, got %d bytes", stdout.Len())
    }
}
```

---

### 30. [ ] Missing test: verify correct plan ID passed to API

**File:** `cmd/tfc/plans_test.go`

**Problem:** Tests verify that API methods are called and return expected results, but don't verify the correct plan ID is passed. After fixing issue #22 (capture parameters), assertions should be added.

**Fix:** After updating `fakePlansClient` per issue #22, add assertions:
```go
// In TestPlansGet_JSON
if fakeClient.readPlanID != "plan-123" {
    t.Errorf("expected plan ID plan-123, got %s", fakeClient.readPlanID)
}

// In TestPlansJSONOutput_WritesToStdout
if fakeClient.jsonOutputPlanID != "plan-123" {
    t.Errorf("expected plan ID plan-123, got %s", fakeClient.jsonOutputPlanID)
}
```

---

### 31. [ ] Missing test: verify download URL passed correctly to `downloadClient`

**File:** `cmd/tfc/plans_test.go`

**Problem:** While `TestPlansSanitizedPlan_NoAuthorizationForwarded` captures and verifies the download URL, other sanitized plan tests don't verify this, missing potential bugs where the wrong URL could be used.

**Fix:** Add URL verification to other sanitized plan tests:
```go
// In TestPlansSanitizedPlan_WritesToFile
var downloadedURL string
cmd := &PlansSanitizedPlanCmd{
    // ... other fields ...
    downloadClient: func(url string) ([]byte, error) {
        downloadedURL = url
        return []byte(sanitizedContent), nil
    },
}
// ... run test ...
if downloadedURL != "https://archivist.example/sanitized.json" {
    t.Errorf("expected download URL, got %s", downloadedURL)
}
```

---

### 32. [ ] `plansClient` interface missing `ReadSanitizedJSON` method

**File:** `cmd/tfc/plans.go`
**Lines:** 52-56

**Problem:** The `plansClient` interface doesn't include a method for reading sanitized plans. Instead, `PlansSanitizedPlanCmd` fetches the plan, extracts the link, and uses a separate HTTP download client. While this works, it creates inconsistency with how other API operations are abstracted.

The TFC API (`go-tfe`) does provide `Plans.ReadSanitizedJSONOutput()` which could be used instead of manual HTTP downloading.

**Note:** This may be intentional if the sanitized plan endpoint requires special handling (no auth header forwarding). Document the reason if this is the case.

**Fix (if standardization is desired):**
```go
type plansClient interface {
    Read(ctx context.Context, planID string) (*tfe.Plan, error)
    ReadJSONOutput(ctx context.Context, planID string) ([]byte, error)
    ReadSanitizedJSONOutput(ctx context.Context, planID string) ([]byte, error) // Add this
}
```

---

### 33. [ ] No integration test for `defaultDownloadClient`

**File:** `cmd/tfc/plans_test.go`

**Problem:** The `defaultDownloadClient` function (lines 377-389 in plans.go) that performs actual HTTP requests is never tested. All tests inject a mock `downloadClient`. This means HTTP error handling, status code checking, and response body reading are untested.

**Fix:** Add an integration-style test using `httptest.Server`:
```go
func TestDefaultDownloadClient(t *testing.T) {
    // Test successful download
    t.Run("success", func(t *testing.T) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte(`{"sanitized":"data"}`))
        }))
        defer server.Close()

        cmd := &PlansSanitizedPlanCmd{
            httpClient: server.Client(),
        }
        cmd.downloadClient = cmd.defaultDownloadClient

        data, err := cmd.downloadClient(server.URL)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if string(data) != `{"sanitized":"data"}` {
            t.Errorf("unexpected data: %s", string(data))
        }
    })

    // Test non-200 status
    t.Run("non200status", func(t *testing.T) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusForbidden)
        }))
        defer server.Close()

        cmd := &PlansSanitizedPlanCmd{
            httpClient: server.Client(),
        }
        cmd.downloadClient = cmd.defaultDownloadClient

        _, err := cmd.downloadClient(server.URL)
        if err == nil {
            t.Fatal("expected error for non-200 status")
        }
        if !strings.Contains(err.Error(), "403") {
            t.Errorf("expected 403 in error, got: %v", err)
        }
    })
}
```

---

# Code Review: Configuration-Versions Subcommand

## Files Reviewed
- `cmd/tfc/configuration_versions.go` - Main implementation (660 lines)
- `cmd/tfc/configuration_versions_test.go` - Unit tests (861 lines)
- `cmd/tfc/common.go` - Shared helpers (resolveFormat, resolveClientConfig)

---

## Issues Found

### 34. [ ] `realCVClient.Upload` ignores the `reader` parameter (BUG)

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 84-86

**Problem:** The `Upload` method receives an `io.Reader` parameter but completely ignores it, passing an empty string to the underlying API call. This means uploads will always fail or upload empty content.

**Current:**
```go
func (c *realCVClient) Upload(ctx context.Context, uploadURL string, reader io.Reader) error {
    return c.client.ConfigurationVersions.Upload(ctx, uploadURL, "")
}
```

**Analysis:** The `go-tfe` library's `Upload` method expects a file path as the second parameter, but the interface signature uses `io.Reader`. This architectural mismatch means either:
1. The interface should accept a file path (string), or
2. A different approach is needed (the code uses `uploadClient` directly anyway)

**Fix:** Since `CVUploadCmd` uses a custom `uploadClient` function instead of the `cvClient.Upload` method, consider either:
1. Removing `Upload` from the interface since it's unused, or
2. Fixing the implementation to match the interface:
```go
func (c *realCVClient) Upload(ctx context.Context, uploadURL string, reader io.Reader) error {
    // Create a temp file from reader, upload it, then clean up
    tmpFile, err := os.CreateTemp("", "cv-upload-*.tar.gz")
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile.Name())
    defer tmpFile.Close()

    if _, err := io.Copy(tmpFile, reader); err != nil {
        return err
    }

    return c.client.ConfigurationVersions.Upload(ctx, uploadURL, tmpFile.Name())
}
```

---

### 35. [ ] Configuration-versions commands don't use `resolveFormat` helper

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 184-189, 257-262, 343-348, 435-440, 540-545, 637-642

**Problem:** All six CV commands duplicate TTY detection logic inline instead of using the `resolveFormat` helper from `common.go`. Other commands like `doctor`, `projects`, `workspace-variables`, and `workspace-resources` use this helper consistently.

**Current code (repeated 6 times):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Replace with:
```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

### 36. [ ] `resolveCVClientConfig` duplicates `resolveClientConfig` from common.go

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 105-138

**Problem:** The `resolveCVClientConfig` function is nearly identical to `resolveClientConfig` in `common.go`, minus the organization resolution. This duplicates ~30 lines of code and creates maintenance burden.

**Current:** Two separate functions with identical context/settings/token resolution logic.

**Fix:** Use `resolveClientConfig` and ignore the org return value:
```go
cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
```

Then delete `resolveCVClientConfig` entirely.

---

### 37. [ ] `defaultUploadClient` doesn't include response body in error

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 477-479

**Problem:** When the HTTP status code indicates failure, the function returns a generic error with just the status code. The response body (which often contains useful error details from the server) is discarded.

**Current:**
```go
if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
    return fmt.Errorf("upload failed with status code: %d", resp.StatusCode)
}
```

**Fix:** Read and include the response body in the error:
```go
if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
    body, _ := io.ReadAll(resp.Body)
    if len(body) > 0 {
        return fmt.Errorf("upload failed with status code %d: %s", resp.StatusCode, string(body))
    }
    return fmt.Errorf("upload failed with status code: %d", resp.StatusCode)
}
```

---

### 38. [ ] `cvJSON` struct missing `created_at` timestamp

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 30-39, 41-52

**Problem:** The `cvJSON` struct doesn't include `CreatedAt` timestamp, but configuration versions have this field. This creates inconsistency with other commands (like runs) that include timestamps in their JSON output.

**Fix:** Add `created_at` to the struct:
```go
type cvJSON struct {
    ID            string `json:"id"`
    Status        string `json:"status"`
    Source        string `json:"source,omitempty"`
    AutoQueueRuns bool   `json:"auto_queue_runs"`
    Speculative   bool   `json:"speculative"`
    ErrorMessage  string `json:"error_message,omitempty"`
    UploadURL     string `json:"upload_url,omitempty"`
    CreatedAt     string `json:"created_at,omitempty"` // Add this
}
```

And update `toCVJSON`:
```go
func toCVJSON(cv *tfe.ConfigurationVersion) *cvJSON {
    r := &cvJSON{
        ID:            cv.ID,
        Status:        string(cv.Status),
        Source:        string(cv.Source),
        AutoQueueRuns: cv.AutoQueueRuns,
        Speculative:   cv.Speculative,
        ErrorMessage:  cv.ErrorMessage,
        UploadURL:     cv.UploadURL,
    }
    if !cv.CreatedAt.IsZero() {
        r.CreatedAt = cv.CreatedAt.Format(time.RFC3339)
    }
    return r
}
```

---

### 39. [ ] `CVDownloadCmd` writes binary content to stdout without TTY warning

**File:** `cmd/tfc/configuration_versions.go`
**Lines:** 567-571

**Problem:** When no `--out` flag is specified, the command writes raw tar.gz binary content directly to stdout. If stdout is a TTY, this can corrupt the terminal display. Many CLI tools warn or refuse to write binary to a TTY.

**Current:**
```go
} else {
    // Write to stdout
    if _, err := c.stdout.Write(content); err != nil {
        return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
    }
}
```

**Fix:** Check if stdout is a TTY and warn or require `--out`:
```go
} else {
    // Check if stdout is a TTY - binary content may corrupt display
    if f, ok := c.stdout.(*os.File); ok && c.ttyDetector.IsTTY(f) {
        return internalcmd.NewRuntimeError(fmt.Errorf("refusing to write binary content to terminal; use --out to specify output file"))
    }
    // Write to stdout
    if _, err := c.stdout.Write(content); err != nil {
        return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
    }
}
```

---

### 40. [ ] Missing test: client factory returns error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error for any CV command.

**Fix:** Add test:
```go
func TestCVList_ClientFactoryError(t *testing.T) {
    tmpDir, resolver := setupCVTest(t)

    var stdout bytes.Buffer
    cmd := &CVListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return nil, errors.New("failed to create TFC client")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for client factory failure")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client error message, got: %v", err)
    }
}
```

---

### 41. [ ] Missing test: invalid context specified via --context flag

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** `TestCVList_FailsWhenSettingsMissing` tests missing settings file, but no test for when `--context` flag specifies a non-existent context.

**Fix:** Add test:
```go
func TestCVList_InvalidContext(t *testing.T) {
    tmpDir, resolver := setupCVTest(t)

    var stdout bytes.Buffer
    cmd := &CVListCmd{
        WorkspaceID:   "ws-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return &fakeCVClient{}, nil
        },
    }

    cli := &CLI{Context: "nonexistent", OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid context")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected context not found error, got: %v", err)
    }
}
```

---

### 42. [ ] Missing test: empty configuration versions list

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies behavior when workspace has zero configuration versions. Both JSON and table output should handle empty results gracefully.

**Fix:** Add test:
```go
func TestCVList_EmptyList(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        ListFunc: func(_ context.Context, _ string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
            return []*tfe.ConfigurationVersion{}, nil // Empty list
        },
    }

    var stdout bytes.Buffer
    cmd := &CVListCmd{
        WorkspaceID:   "ws-empty",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var result map[string]any
    if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
        t.Fatalf("failed to parse JSON: %v", err)
    }

    data, ok := result["data"].([]any)
    if !ok {
        t.Fatalf("expected data array, got %T", result["data"])
    }
    if len(data) != 0 {
        t.Errorf("expected 0 CVs, got %d", len(data))
    }
}
```

---

### 43. [ ] Missing test: CVCreate API error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** While `TestCVList_APIError` and `TestCVGet_NotFound` test API failures for those commands, there's no equivalent for `CVCreateCmd`.

**Fix:** Add test:
```go
func TestCVCreate_APIError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        CreateFunc: func(_ context.Context, _ string, _ tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
            return nil, errors.New("workspace not found")
        },
    }

    var stdout bytes.Buffer
    cmd := &CVCreateCmd{
        WorkspaceID:   "ws-invalid",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for API failure")
    }
    if !strings.Contains(err.Error(), "workspace not found") {
        t.Errorf("expected error message, got: %v", err)
    }
}
```

---

### 44. [ ] Missing test: CVUpload file read error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when the file specified by `--file` cannot be read (doesn't exist, permission denied, etc.).

**Fix:** Add test:
```go
func TestCVUpload_FileReadError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
            return &tfe.ConfigurationVersion{
                ID:        cvID,
                Status:    tfe.ConfigurationPending,
                UploadURL: "https://archivist.example.com/upload",
            }, nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVUploadCmd{
        ID:            "cv-123",
        File:          "/nonexistent/path/config.tar.gz",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
        fileReader: func(path string) ([]byte, error) {
            return nil, os.ErrNotExist
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for file read failure")
    }
    if !strings.Contains(err.Error(), "failed to read file") {
        t.Errorf("expected file read error, got: %v", err)
    }
}
```

---

### 45. [ ] Missing test: CVDownload file write error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when `os.WriteFile` fails for `CVDownloadCmd` (e.g., writing to a non-existent directory).

**Fix:** Add test:
```go
func TestCVDownload_FileWriteError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        DownloadFunc: func(_ context.Context, _ string) ([]byte, error) {
            return []byte("downloaded-content"), nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVDownloadCmd{
        ID:            "cv-123",
        Out:           "/nonexistent/directory/output.tar.gz", // Invalid path
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid file path")
    }
    if !strings.Contains(err.Error(), "failed to write file") {
        t.Errorf("expected file write error, got: %v", err)
    }
}
```

---

### 46. [ ] Missing test: CVArchive prompter error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** While tests verify both "yes" and "no" confirmation paths, there's no test for when `prompter.Confirm` returns an error (e.g., stdin closed).

**Fix:** Add test:
```go
type cvErrorPrompter struct{}

func (p *cvErrorPrompter) PromptString(_, _ string) (string, error) {
    return "", errors.New("stdin closed")
}

func (p *cvErrorPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, errors.New("stdin closed")
}

func (p *cvErrorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", errors.New("stdin closed")
}

func TestCVArchive_PrompterError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    archiveCalled := false
    fakeClient := &fakeCVClient{
        ArchiveFunc: func(_ context.Context, _ string) error {
            archiveCalled = true
            return nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVArchiveCmd{
        ID:            "cv-123",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
        prompter: &cvErrorPrompter{},
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for prompter failure")
    }
    if !strings.Contains(err.Error(), "failed to prompt") {
        t.Errorf("expected prompt error, got: %v", err)
    }
    if archiveCalled {
        t.Error("archive should not be called when prompt fails")
    }
}
```

---

### 47. [ ] Missing test: uploadClient HTTP error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when the `uploadClient` function fails (network error, HTTP error status, etc.).

**Fix:** Add test:
```go
func TestCVUpload_UploadClientError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
            return &tfe.ConfigurationVersion{
                ID:        cvID,
                Status:    tfe.ConfigurationPending,
                UploadURL: "https://archivist.example.com/upload",
            }, nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVUploadCmd{
        ID:            "cv-123",
        File:          "/path/to/config.tar.gz",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
        fileReader: func(path string) ([]byte, error) {
            return []byte("fake-content"), nil
        },
        uploadClient: func(url string, content []byte) error {
            return errors.New("upload failed with status code: 403")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for upload failure")
    }
    if !strings.Contains(err.Error(), "failed to upload") {
        t.Errorf("expected upload error, got: %v", err)
    }
}
```

---

### 48. [ ] Missing test: CVGet table output

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** There's `TestCVGet_JSON` but no test for table output format, which has different logic (field/value pairs with conditional rows for ErrorMessage and UploadURL).

**Fix:** Add test:
```go
func TestCVGet_Table(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
            return &tfe.ConfigurationVersion{
                ID:            "cv-123",
                Status:        tfe.ConfigurationErrored,
                Source:        tfe.ConfigurationSourceAPI,
                AutoQueueRuns: false,
                Speculative:   true,
                ErrorMessage:  "Invalid HCL syntax",
                UploadURL:     "", // No upload URL for errored state
            }, nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVGetCmd{
        ID:            "cv-123",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := stdout.String()
    if !strings.Contains(out, "cv-123") {
        t.Errorf("expected CV ID in output, got: %s", out)
    }
    if !strings.Contains(out, "Error Message") {
        t.Errorf("expected Error Message field for errored CV, got: %s", out)
    }
    if !strings.Contains(out, "Invalid HCL syntax") {
        t.Errorf("expected error message content, got: %s", out)
    }
}
```

---

### 49. [ ] Missing test: CVCreate table output

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** `TestCVCreate_JSON` and `TestCVCreate_WithSpeculative` only test JSON output. No test verifies the table/text output format.

**Fix:** Add test:
```go
func TestCVCreate_TableOutput(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        CreateFunc: func(_ context.Context, _ string, _ tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
            return &tfe.ConfigurationVersion{
                ID:        "cv-new",
                Status:    tfe.ConfigurationPending,
                UploadURL: "https://archivist.example.com/upload/cv-new",
            }, nil
        },
    }

    var stdout bytes.Buffer
    cmd := &CVCreateCmd{
        WorkspaceID:   "ws-123",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := stdout.String()
    if !strings.Contains(out, "cv-new") {
        t.Errorf("expected CV ID in output, got: %s", out)
    }
    if !strings.Contains(out, "created") {
        t.Errorf("expected 'created' message, got: %s", out)
    }
    if !strings.Contains(out, "Upload URL") {
        t.Errorf("expected Upload URL in output, got: %s", out)
    }
}
```

---

### 50. [ ] No integration test for `defaultUploadClient`

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** The `defaultUploadClient` function (lines 461-482) that performs actual HTTP PUT requests is never tested. All tests inject a mock `uploadClient`. This means HTTP error handling, status code checking, and response body reading are untested.

**Fix:** Add an integration-style test using `httptest.Server`:
```go
func TestDefaultUploadClient(t *testing.T) {
    // Test successful upload
    t.Run("success", func(t *testing.T) {
        var receivedContent []byte
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.Method != http.MethodPut {
                t.Errorf("expected PUT, got %s", r.Method)
            }
            receivedContent, _ = io.ReadAll(r.Body)
            w.WriteHeader(http.StatusOK)
        }))
        defer server.Close()

        content := []byte("test-upload-content")
        err := defaultUploadClient(server.URL, content)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if string(receivedContent) != string(content) {
            t.Errorf("expected content %q, got %q", string(content), string(receivedContent))
        }
    })

    // Test non-success status
    t.Run("non200status", func(t *testing.T) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusForbidden)
        }))
        defer server.Close()

        err := defaultUploadClient(server.URL, []byte("test"))
        if err == nil {
            t.Fatal("expected error for non-200 status")
        }
        if !strings.Contains(err.Error(), "403") {
            t.Errorf("expected 403 in error, got: %v", err)
        }
    })

    // Test 201 Created is accepted
    t.Run("status201", func(t *testing.T) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusCreated)
        }))
        defer server.Close()

        err := defaultUploadClient(server.URL, []byte("test"))
        if err != nil {
            t.Fatalf("unexpected error for 201 status: %v", err)
        }
    })

    // Test 204 No Content is accepted
    t.Run("status204", func(t *testing.T) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusNoContent)
        }))
        defer server.Close()

        err := defaultUploadClient(server.URL, []byte("test"))
        if err != nil {
            t.Fatalf("unexpected error for 204 status: %v", err)
        }
    })
}
```

---

### 51. [ ] Missing test: CVArchive API error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when the Archive API call fails.

**Fix:** Add test:
```go
func TestCVArchive_APIError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        ArchiveFunc: func(_ context.Context, _ string) error {
            return errors.New("configuration version is in use by an active run")
        },
    }

    var stdout bytes.Buffer
    cmd := &CVArchiveCmd{
        ID:            "cv-123",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{Force: true, OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for API failure")
    }
    if !strings.Contains(err.Error(), "configuration version is in use") {
        t.Errorf("expected specific error message, got: %v", err)
    }
}
```

---

### 52. [ ] Missing test: CVDownload API error

**File:** `cmd/tfc/configuration_versions_test.go`

**Problem:** No test verifies error handling when the Download API call fails.

**Fix:** Add test:
```go
func TestCVDownload_APIError(t *testing.T) {
    baseDir, resolver := setupCVTest(t)

    fakeClient := &fakeCVClient{
        DownloadFunc: func(_ context.Context, _ string) ([]byte, error) {
            return nil, errors.New("configuration version not found")
        },
    }

    var stdout bytes.Buffer
    cmd := &CVDownloadCmd{
        ID:            "cv-nonexistent",
        baseDir:       baseDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for API failure")
    }
    if !strings.Contains(err.Error(), "configuration version not found") {
        t.Errorf("expected error message, got: %v", err)
    }
}
```

---

# Code Review: Users Subcommand

## Files Reviewed
- `cmd/tfc/users.go` - Main implementation (232 lines)
- `cmd/tfc/users_test.go` - Unit tests (406 lines)
- `cmd/tfc/common.go` - Shared helpers (resolveFormat, resolveClientConfig)

---

## Issues Found

### 53. [ ] Users command doesn't use `resolveFormat` helper

**File:** `cmd/tfc/users.go`
**Lines:** 206-210

**Problem:** The `UsersGetCmd` duplicates TTY detection logic inline instead of using the `resolveFormat` helper from `common.go`. Other commands like `doctor`, `projects`, `workspace-variables`, and `workspace-resources` use this helper consistently.

**Current code:**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Replace with:
```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

### 54. [ ] `resolveUsersClientConfig` duplicates `resolveClientConfig` from common.go

**File:** `cmd/tfc/users.go`
**Lines:** 127-159

**Problem:** The `resolveUsersClientConfig` function is nearly identical to `resolveClientConfig` in `common.go`, minus the organization resolution. This duplicates ~30 lines of code and creates maintenance burden.

**Current:** Two separate functions with identical context/settings/token resolution logic.

**Fix:** Use `resolveClientConfig` and ignore the org return value:
```go
cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
```

Then delete `resolveUsersClientConfig` entirely.

---

### 55. [ ] `fakeUsersClient` doesn't capture `userID` for verification

**File:** `cmd/tfc/users_test.go`
**Lines:** 17-28

**Problem:** The fake client ignores the `userID` parameter in `Read`, making it impossible to verify that commands pass the correct user ID to the API.

**Current:**
```go
func (c *fakeUsersClient) Read(_ context.Context, _ string) (*UserResponse, error) {
    if c.err != nil {
        return nil, c.err
    }
    return c.user, nil
}
```

**Fix:** Add field to capture parameter:
```go
type fakeUsersClient struct {
    user *UserResponse
    err  error

    // Captured parameters for verification
    readUserID string
}

func (c *fakeUsersClient) Read(_ context.Context, userID string) (*UserResponse, error) {
    c.readUserID = userID
    if c.err != nil {
        return nil, c.err
    }
    return c.user, nil
}
```

Then add assertions in tests:
```go
// In TestUsersGet_JSON
if fakeClient.readUserID != "user-abc123" {
    t.Errorf("expected user ID user-abc123, got %s", fakeClient.readUserID)
}
```

---

### 56. [ ] Missing test: client factory returns error

**File:** `cmd/tfc/users_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error.

**Fix:** Add test:
```go
func TestUsersGet_ClientFactoryError(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    var stdout bytes.Buffer
    cmd := &UsersGetCmd{
        UserID:        "user-123",
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
            return nil, errors.New("failed to create TFC client")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for client factory failure")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client error message, got: %v", err)
    }
}
```

---

### 57. [ ] Missing test: invalid context specified via --context flag

**File:** `cmd/tfc/users_test.go`

**Problem:** `TestUsersGet_FailsWhenSettingsMissing` tests missing settings file, but no test for when `--context` flag specifies a non-existent context.

**Fix:** Add test:
```go
func TestUsersGet_InvalidContext(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    var stdout bytes.Buffer
    cmd := &UsersGetCmd{
        UserID:        "user-123",
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
            return &fakeUsersClient{}, nil
        },
    }

    cli := &CLI{Context: "nonexistent", OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid context")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected context not found error, got: %v", err)
    }
}
```

---

### 58. [ ] `realUsersClient.Read` doesn't validate empty userID

**File:** `cmd/tfc/users.go`
**Lines:** 61-110

**Problem:** If an empty string is passed as `userID`, the function will make a request to `/api/v2/users/` (note: trailing slash with empty ID). This may return an unexpected response or error from the API. Validating input early provides clearer error messages.

**Current:**
```go
func (c *realUsersClient) Read(ctx context.Context, userID string) (*UserResponse, error) {
    apiURL := fmt.Sprintf("%s/api/v2/users/%s", c.baseURL, url.PathEscape(userID))
    // ...
}
```

**Fix:** Add validation at the start of the function:
```go
func (c *realUsersClient) Read(ctx context.Context, userID string) (*UserResponse, error) {
    if userID == "" {
        return nil, fmt.Errorf("user ID is required")
    }
    apiURL := fmt.Sprintf("%s/api/v2/users/%s", c.baseURL, url.PathEscape(userID))
    // ...
}
```

---

### 59. [ ] `V2Only` field not displayed in table output

**File:** `cmd/tfc/users.go`
**Lines:** 47, 218-227

**Problem:** The `UserAttributes` struct includes `V2Only` field (line 47), and JSON output includes it. However, table output (lines 218-227) doesn't display `V2Only`, creating inconsistency between JSON and table formats.

**Current table output:**
```go
tw.AddRow("ID", user.Data.ID)
tw.AddRow("Username", user.Data.Attributes.Username)
tw.AddRow("Email", user.Data.Attributes.Email)
tw.AddRow("Avatar URL", user.Data.Attributes.AvatarURL)
tw.AddRow("Service Account", fmt.Sprintf("%t", user.Data.Attributes.IsServiceAccount))
```

**Fix:** Add `V2Only` to table output for consistency:
```go
tw.AddRow("ID", user.Data.ID)
tw.AddRow("Username", user.Data.Attributes.Username)
tw.AddRow("Email", user.Data.Attributes.Email)
tw.AddRow("Avatar URL", user.Data.Attributes.AvatarURL)
tw.AddRow("Service Account", fmt.Sprintf("%t", user.Data.Attributes.IsServiceAccount))
tw.AddRow("V2 Only", fmt.Sprintf("%t", user.Data.Attributes.V2Only))
```

---

### 60. [ ] `realUsersClient.Read` doesn't handle 403 Forbidden status separately

**File:** `cmd/tfc/users.go`
**Lines:** 83-101

**Problem:** The function handles 404 (Not Found) and 401 (Unauthorized) with specific error messages, but 403 (Forbidden) falls through to the generic error handler. A 403 typically means the user exists but the authenticated user lacks permission to view them. Other commands like `invoices.go` handle 403 specifically.

**Current:**
```go
if resp.StatusCode == http.StatusNotFound {
    return nil, fmt.Errorf("user not found: %s", userID)
}
if resp.StatusCode == http.StatusUnauthorized {
    return nil, fmt.Errorf("unauthorized: invalid or missing API token")
}
if resp.StatusCode != http.StatusOK {
    // Generic error handling...
}
```

**Fix:** Add specific handling for 403:
```go
if resp.StatusCode == http.StatusNotFound {
    return nil, fmt.Errorf("user not found: %s", userID)
}
if resp.StatusCode == http.StatusUnauthorized {
    return nil, fmt.Errorf("unauthorized: invalid or missing API token")
}
if resp.StatusCode == http.StatusForbidden {
    return nil, fmt.Errorf("forbidden: insufficient permissions to view user %s", userID)
}
if resp.StatusCode != http.StatusOK {
    // Generic error handling...
}
```

---

### 61. [ ] Missing test: service account user

**File:** `cmd/tfc/users_test.go`

**Problem:** No test verifies the output when `IsServiceAccount` is `true`. This is an important field that distinguishes human users from service accounts.

**Fix:** Add test:
```go
func TestUsersGet_ServiceAccount(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    user := &UserResponse{
        Data: UserData{
            ID:   "user-svc123",
            Type: "users",
            Attributes: UserAttributes{
                Username:         "my-service-account",
                Email:            "", // Service accounts often have no email
                IsServiceAccount: true,
                V2Only:           true,
            },
        },
    }

    var stdout bytes.Buffer
    cmd := &UsersGetCmd{
        UserID:        "user-svc123",
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
            return &fakeUsersClient{user: user}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    var result UserResponse
    if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
        t.Fatalf("failed to parse JSON output: %v", err)
    }

    if !result.Data.Attributes.IsServiceAccount {
        t.Error("expected IsServiceAccount to be true")
    }
    if result.Data.Attributes.Email != "" {
        t.Errorf("expected empty email for service account, got %q", result.Data.Attributes.Email)
    }
}
```

---

### 62. [ ] Missing test: empty email and avatar fields

**File:** `cmd/tfc/users_test.go`

**Problem:** The `UserAttributes` struct uses `omitempty` for Email and AvatarURL, suggesting these can be empty. No test verifies this displays correctly in both JSON and table output.

**Fix:** Add test:
```go
func TestUsersGet_EmptyOptionalFields(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    user := &UserResponse{
        Data: UserData{
            ID:   "user-minimal",
            Type: "users",
            Attributes: UserAttributes{
                Username:         "minimaluser",
                Email:            "", // Empty
                AvatarURL:        "", // Empty
                IsServiceAccount: false,
            },
        },
    }

    t.Run("JSON", func(t *testing.T) {
        var stdout bytes.Buffer
        cmd := &UsersGetCmd{
            UserID:        "user-minimal",
            baseDir:       baseDir,
            tokenResolver: tokenResolver,
            ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
            stdout:        &stdout,
            clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
                return &fakeUsersClient{user: user}, nil
            },
        }

        cli := &CLI{OutputFormat: "json"}
        err := cmd.Run(cli)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }

        // Verify omitempty works - empty fields should not appear in JSON
        output := stdout.String()
        if strings.Contains(output, `"email":""`) {
            t.Error("expected empty email to be omitted from JSON")
        }
    })

    t.Run("Table", func(t *testing.T) {
        var stdout bytes.Buffer
        cmd := &UsersGetCmd{
            UserID:        "user-minimal",
            baseDir:       baseDir,
            tokenResolver: tokenResolver,
            ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
            stdout:        &stdout,
            clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
                return &fakeUsersClient{user: user}, nil
            },
        }

        cli := &CLI{OutputFormat: "table"}
        err := cmd.Run(cli)
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }

        // Table should still show the row even if empty
        out := stdout.String()
        if !strings.Contains(out, "Email") {
            t.Error("expected Email field in table output")
        }
    })
}
```

---

### 63. [ ] Missing test: JSON:API error response parsing

**File:** `cmd/tfc/users_test.go`

**Problem:** Lines 90-100 in `users.go` attempt to parse JSON:API error responses from the server, but there's no test verifying this code path works correctly. This error parsing logic could silently fail.

**Fix:** Add test that verifies JSON:API error details are extracted:
```go
func TestUsersGet_JSONAPIErrorResponse(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    // This test requires testing the realUsersClient with httptest
    // or creating an error type that simulates JSON:API errors

    var stdout bytes.Buffer
    cmd := &UsersGetCmd{
        UserID:        "user-err",
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
            // Simulate a JSON:API formatted error
            return &fakeUsersClient{
                err: errors.New("Invalid user ID: user ID must start with 'user-'"),
            }, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error")
    }

    errStr := err.Error()
    if !strings.Contains(errStr, "Invalid user ID") {
        t.Errorf("expected detailed error message, got: %s", errStr)
    }
}
```

---

### 64. [ ] Missing test: malformed JSON response from API

**File:** `cmd/tfc/users_test.go`

**Problem:** No test verifies error handling when the API returns invalid JSON that cannot be parsed. Lines 104-107 in `users.go` handle this, but the path is untested.

**Fix:** Since this requires testing the real HTTP client, add an integration-style test using `httptest.Server`:
```go
func TestRealUsersClient_MalformedJSON(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{invalid json`))
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.Read(context.Background(), "user-123")
    if err == nil {
        t.Fatal("expected error for malformed JSON")
    }
    if !strings.Contains(err.Error(), "failed to parse response") {
        t.Errorf("expected parse error, got: %v", err)
    }
}
```

---

### 65. [ ] Missing test: output write failure

**File:** `cmd/tfc/users_test.go`

**Problem:** No test verifies error handling when writing to stdout fails. Lines 214-216 and 225-227 in `users.go` handle write errors, but this path is untested.

**Fix:** Add test with a writer that fails:
```go
type failingWriter struct {
    failAfter int
    written   int
}

func (w *failingWriter) Write(p []byte) (int, error) {
    if w.written >= w.failAfter {
        return 0, errors.New("write failed: disk full")
    }
    w.written += len(p)
    return len(p), nil
}

func TestUsersGet_OutputWriteError(t *testing.T) {
    baseDir, tokenResolver := setupUsersTestSettings(t)

    user := &UserResponse{
        Data: UserData{
            ID:   "user-123",
            Type: "users",
            Attributes: UserAttributes{
                Username: "testuser",
            },
        },
    }

    cmd := &UsersGetCmd{
        UserID:        "user-123",
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &failingWriter{failAfter: 0}, // Fail immediately
        clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
            return &fakeUsersClient{user: user}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for write failure")
    }
    if !strings.Contains(err.Error(), "failed to write output") {
        t.Errorf("expected write error, got: %v", err)
    }
}
```

---

### 66. [ ] No integration test for `realUsersClient`

**File:** `cmd/tfc/users_test.go`

**Problem:** The `realUsersClient` struct and its HTTP handling logic are never directly tested. All tests inject `fakeUsersClient`. This means HTTP request construction, header setting, status code handling, and response parsing are untested.

**Fix:** Add integration-style tests using `httptest.Server`:
```go
func TestRealUsersClient_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.Method != "GET" {
            t.Errorf("expected GET, got %s", r.Method)
        }
        if r.URL.Path != "/api/v2/users/user-123" {
            t.Errorf("expected path /api/v2/users/user-123, got %s", r.URL.Path)
        }
        if r.Header.Get("Authorization") != "Bearer test-token" {
            t.Errorf("expected Bearer token, got %s", r.Header.Get("Authorization"))
        }
        if r.Header.Get("Content-Type") != "application/vnd.api+json" {
            t.Errorf("expected JSON:API content type, got %s", r.Header.Get("Content-Type"))
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(UserResponse{
            Data: UserData{
                ID:   "user-123",
                Type: "users",
                Attributes: UserAttributes{
                    Username: "testuser",
                    Email:    "test@example.com",
                },
            },
        })
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    user, err := client.Read(context.Background(), "user-123")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.Data.ID != "user-123" {
        t.Errorf("expected user-123, got %s", user.Data.ID)
    }
}

func TestRealUsersClient_NotFound(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNotFound)
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.Read(context.Background(), "user-404")
    if err == nil {
        t.Fatal("expected error for 404")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected not found error, got: %v", err)
    }
}

func TestRealUsersClient_Unauthorized(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "bad-token",
        httpClient: server.Client(),
    }

    _, err := client.Read(context.Background(), "user-123")
    if err == nil {
        t.Fatal("expected error for 401")
    }
    if !strings.Contains(err.Error(), "unauthorized") {
        t.Errorf("expected unauthorized error, got: %v", err)
    }
}

func TestRealUsersClient_JSONAPIError(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{
            "errors": [{
                "status": "400",
                "title": "Bad Request",
                "detail": "Invalid user ID format"
            }]
        }`))
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.Read(context.Background(), "invalid")
    if err == nil {
        t.Fatal("expected error for 400")
    }
    if !strings.Contains(err.Error(), "Bad Request") || !strings.Contains(err.Error(), "Invalid user ID format") {
        t.Errorf("expected JSON:API error details, got: %v", err)
    }
}
```

---

### 67. [ ] Missing test: verify correct user ID passed to Read

**File:** `cmd/tfc/users_test.go`

**Problem:** Tests verify the API methods are called and return expected results, but don't verify the correct user ID is passed. After fixing issue #55 (capture parameters), assertions should be added.

**Fix:** After updating `fakeUsersClient` per issue #55, add assertions to existing tests:
```go
// In TestUsersGet_JSON, after running cmd.Run():
if fakeClient.readUserID != "user-abc123" {
    t.Errorf("expected user ID user-abc123, got %s", fakeClient.readUserID)
}
```

Note: This requires modifying tests to keep a reference to the fakeClient:
```go
fakeClient := &fakeUsersClient{user: user}
cmd := &UsersGetCmd{
    // ...
    clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
        return fakeClient, nil
    },
}
// Run test...
if fakeClient.readUserID != "user-abc123" {
    t.Errorf(...)
}
```

---

### 68. [ ] `realUsersClient` creates new http.Client for each command invocation

**File:** `cmd/tfc/users.go`
**Line:** 122

**Problem:** The `defaultUsersClientFactory` creates a new `http.Client{}` for each command invocation. While not a bug for CLI usage, this prevents connection reuse and could impact performance for batch operations or scripts that invoke the command multiple times.

**Current:**
```go
return &realUsersClient{
    baseURL:    baseURL,
    token:      cfg.Token,
    httpClient: &http.Client{},
}, nil
```

**Note:** This is a minor issue for CLI tools. However, if a timeout is needed, it should be configured:
```go
httpClient: &http.Client{
    Timeout: 30 * time.Second,
},
```

---

### 69. [ ] Missing test: userID with special characters

**File:** `cmd/tfc/users_test.go`

**Problem:** Line 62 in `users.go` uses `url.PathEscape(userID)` to escape the user ID in the URL path. However, no test verifies this escaping works correctly for user IDs with special characters that need escaping.

**Fix:** Add test:
```go
func TestRealUsersClient_UserIDWithSpecialChars(t *testing.T) {
    var requestedPath string
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestedPath = r.URL.Path
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(UserResponse{
            Data: UserData{ID: "user-123", Type: "users"},
        })
    }))
    defer server.Close()

    client := &realUsersClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    // Test with a user ID that has characters needing escaping
    _, err := client.Read(context.Background(), "user/with/slashes")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify slashes were escaped
    if requestedPath != "/api/v2/users/user%2Fwith%2Fslashes" {
        t.Errorf("expected escaped path, got %s", requestedPath)
    }
}
```

---

# Code Review: Invoices Subcommand

## Files Reviewed
- `cmd/tfc/invoices.go` - Main implementation (442 lines)
- `cmd/tfc/invoices_test.go` - Unit tests (662 lines)
- `cmd/tfc/common.go` - Shared helpers (resolveFormat, resolveClientConfig)

---

## Issues Found

### 70. [ ] Invoices commands don't use `resolveFormat` helper

**File:** `cmd/tfc/invoices.go`
**Lines:** 316-320, 404-409

**Problem:** Both `InvoicesListCmd` and `InvoicesNextCmd` duplicate TTY detection logic inline instead of using the `resolveFormat` helper from `common.go`. Other commands like `doctor`, `projects`, `workspace-variables`, and `workspace-resources` use this helper consistently.

**Current code (repeated 2 times):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:** Replace with:
```go
format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

---

### 71. [ ] `resolveInvoicesClientConfig` duplicates `resolveClientConfig` from common.go

**File:** `cmd/tfc/invoices.go`
**Lines:** 223-262

**Problem:** The `resolveInvoicesClientConfig` function is nearly identical to `resolveClientConfig` in `common.go`. This duplicates ~40 lines of code and creates maintenance burden.

**Current:** Two separate functions with identical context/settings/token/org resolution logic.

**Fix:** Use `resolveClientConfig` directly:
```go
cfg, org, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
```

Then delete `resolveInvoicesClientConfig` entirely.

---

### 72. [ ] `realInvoicesClient.List` pagination loop lacks infinite loop protection

**File:** `cmd/tfc/invoices.go`
**Lines:** 82-123

**Problem:** The pagination loop in `List` continues while `listResp.Meta.Continuation` is non-empty. If the API incorrectly returns the same continuation token repeatedly (due to a bug or network issue), this creates an infinite loop. There's no maximum iteration count or duplicate token detection.

**Current:**
```go
for {
    // ... make request ...
    if listResp.Meta == nil || listResp.Meta.Continuation == "" {
        break
    }
    continuation = listResp.Meta.Continuation
}
```

**Fix:** Add safeguards:
```go
const maxPages = 100 // Reasonable limit
seen := make(map[string]bool)
for page := 0; page < maxPages; page++ {
    // ... make request ...
    if listResp.Meta == nil || listResp.Meta.Continuation == "" {
        break
    }
    if seen[listResp.Meta.Continuation] {
        // Duplicate token - API bug, break to avoid infinite loop
        break
    }
    seen[listResp.Meta.Continuation] = true
    continuation = listResp.Meta.Continuation
}
```

---

### 73. [ ] `realInvoicesClient` missing HTTP timeout

**File:** `cmd/tfc/invoices.go`
**Line:** 219

**Problem:** The `defaultInvoicesClientFactory` creates an `http.Client{}` with no timeout configured. This means requests could hang indefinitely if the server doesn't respond.

**Current:**
```go
return &realInvoicesClient{
    baseURL:    baseURL,
    token:      cfg.Token,
    httpClient: &http.Client{},
}, nil
```

**Fix:** Add a reasonable timeout:
```go
return &realInvoicesClient{
    baseURL:    baseURL,
    token:      cfg.Token,
    httpClient: &http.Client{
        Timeout: 30 * time.Second,
    },
}, nil
```

---

### 74. [ ] `handleErrorResponse` uses fragile string matching for "invoices not available"

**File:** `cmd/tfc/invoices.go`
**Lines:** 168-175

**Problem:** The code checks for "invoices not available" by doing string matching on the response body (`strings.Contains(string(body), "invoices")`). This is fragile - any response containing the word "invoices" could be misclassified. The error detection should be more precise.

**Current:**
```go
if statusCode == http.StatusNotFound {
    if strings.Contains(string(body), "invoices") ||
        strings.Contains(string(body), "not found") ||
        strings.Contains(string(body), "Not Found") {
        return &invoicesNotAvailableError{}
    }
    return fmt.Errorf("resource not found")
}
```

**Fix:** Parse the JSON:API error structure first, then check for specific error codes/titles:
```go
if statusCode == http.StatusNotFound {
    var errResp struct {
        Errors []struct {
            Status string `json:"status"`
            Title  string `json:"title"`
            Detail string `json:"detail"`
        } `json:"errors"`
    }
    if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
        // Check for specific API error indicating invoices not available
        for _, e := range errResp.Errors {
            if strings.Contains(strings.ToLower(e.Title), "not found") ||
               strings.Contains(strings.ToLower(e.Detail), "invoices") {
                return &invoicesNotAvailableError{}
            }
        }
        return fmt.Errorf("%s: %s", errResp.Errors[0].Title, errResp.Errors[0].Detail)
    }
    return fmt.Errorf("resource not found")
}
```

---

### 75. [ ] `InvoicesList` table output doesn't include ExternalLink

**File:** `cmd/tfc/invoices.go`
**Lines:** 329-344

**Problem:** The `InvoicesListCmd` table output doesn't include the `ExternalLink` field, but the JSON output does. The `InvoicesNextCmd` table output (lines 432-434) conditionally shows ExternalLink. This creates inconsistency between commands and formats.

**Current table columns:**
```go
tw := output.NewTableWriter(c.stdout, []string{"ID", "STATUS", "NUMBER", "TOTAL", "PAID", "CREATED"}, isTTY)
```

**Fix:** Add ExternalLink column to list table output:
```go
tw := output.NewTableWriter(c.stdout, []string{"ID", "STATUS", "NUMBER", "TOTAL", "PAID", "CREATED", "EXTERNAL LINK"}, isTTY)
for _, inv := range invoices.Data {
    // ... existing code ...
    externalLink := inv.Attributes.ExternalLink
    if externalLink == "" {
        externalLink = "-"
    }
    tw.AddRow(
        inv.ID,
        inv.Attributes.Status,
        inv.Attributes.Number,
        totalDollars,
        paid,
        inv.Attributes.CreatedAt.Format("2006-01-02"),
        externalLink,
    )
}
```

---

### 76. [ ] `fakeInvoicesClient` doesn't capture parameters for verification

**File:** `cmd/tfc/invoices_test.go`
**Lines:** 19-37

**Problem:** The fake client ignores the `org` parameter in both `List` and `GetNext`, making it impossible to verify that commands pass the correct organization to the API.

**Current:**
```go
func (c *fakeInvoicesClient) List(_ context.Context, _ string) (*InvoicesListResponse, error) {
    if c.err != nil {
        return nil, c.err
    }
    return c.listResponse, nil
}
```

**Fix:** Add fields to capture parameters:
```go
type fakeInvoicesClient struct {
    listResponse *InvoicesListResponse
    nextResponse *InvoiceResponse
    err          error

    // Captured parameters for verification
    listOrg    string
    getNextOrg string
}

func (c *fakeInvoicesClient) List(_ context.Context, org string) (*InvoicesListResponse, error) {
    c.listOrg = org
    if c.err != nil {
        return nil, c.err
    }
    return c.listResponse, nil
}

func (c *fakeInvoicesClient) GetNext(_ context.Context, org string) (*InvoiceResponse, error) {
    c.getNextOrg = org
    if c.err != nil {
        return nil, c.err
    }
    return c.nextResponse, nil
}
```

---

### 77. [ ] `TestInvoicesList_UsesOrgFlag` doesn't actually verify captured org

**File:** `cmd/tfc/invoices_test.go`
**Lines:** 262-297

**Problem:** The test creates an `orgCapturingInvoicesClient` and captures the org, but never asserts that the captured value equals "custom-org". The comment on line 295-296 acknowledges this: "we'd need to verify via the client call".

**Current:**
```go
cli := &CLI{Org: "custom-org", OutputFormat: "json"}
err := cmd.Run(cli)
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
// The org is used in the command, but we'd need to verify via the client call
// For this test, we just verify it doesn't error
```

**Fix:** Add assertion:
```go
cli := &CLI{Org: "custom-org", OutputFormat: "json"}
err := cmd.Run(cli)
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}

if capturedOrg != "custom-org" {
    t.Errorf("expected org 'custom-org', got %q", capturedOrg)
}
```

---

### 78. [ ] Missing test: client factory returns error

**File:** `cmd/tfc/invoices_test.go`

**Problem:** No test verifies error handling when `clientFactory` returns an error.

**Fix:** Add test:
```go
func TestInvoicesList_ClientFactoryError(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    var stdout bytes.Buffer
    cmd := &InvoicesListCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return nil, errors.New("failed to create TFC client")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for client factory failure")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client error message, got: %v", err)
    }
}
```

---

### 79. [ ] Missing test: invalid context specified via --context flag

**File:** `cmd/tfc/invoices_test.go`

**Problem:** `TestInvoicesList_FailsWhenSettingsMissing` tests missing settings file, but no test for when `--context` flag specifies a non-existent context.

**Fix:** Add test:
```go
func TestInvoicesList_InvalidContext(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    var stdout bytes.Buffer
    cmd := &InvoicesListCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return &fakeInvoicesClient{}, nil
        },
    }

    cli := &CLI{Context: "nonexistent", OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for invalid context")
    }
    if !strings.Contains(err.Error(), "not found") {
        t.Errorf("expected context not found error, got: %v", err)
    }
}
```

---

### 80. [ ] Missing test: pagination with continuation token

**File:** `cmd/tfc/invoices_test.go`

**Problem:** No test verifies that the pagination loop in `List` correctly handles continuation tokens. The implementation has a pagination loop (lines 82-123 in invoices.go), but there's no test that exercises multiple pages.

**Fix:** Add integration test using `httptest.Server`:
```go
func TestRealInvoicesClient_Pagination(t *testing.T) {
    page := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        page++
        var resp InvoicesListResponse
        if page == 1 {
            resp = InvoicesListResponse{
                Data: []InvoiceData{{ID: "inv-1", Type: "billing-invoices"}},
                Meta: &InvoiceListMeta{Continuation: "cursor-page2"},
            }
        } else {
            resp = InvoicesListResponse{
                Data: []InvoiceData{{ID: "inv-2", Type: "billing-invoices"}},
                Meta: nil, // No more pages
            }
        }
        json.NewEncoder(w).Encode(resp)
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    result, err := client.List(context.Background(), "test-org")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(result.Data) != 2 {
        t.Errorf("expected 2 invoices from 2 pages, got %d", len(result.Data))
    }
    if page != 2 {
        t.Errorf("expected 2 page requests, got %d", page)
    }
}
```

---

### 81. [ ] Missing test: `handleErrorResponse` JSON:API error parsing

**File:** `cmd/tfc/invoices_test.go`

**Problem:** Lines 187-197 in `invoices.go` attempt to parse JSON:API error responses, but there's no test verifying this code path works correctly.

**Fix:** Add integration test:
```go
func TestRealInvoicesClient_JSONAPIError(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{
            "errors": [{
                "status": "400",
                "title": "Bad Request",
                "detail": "Invalid organization name"
            }]
        }`))
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.List(context.Background(), "invalid-org")
    if err == nil {
        t.Fatal("expected error for 400")
    }
    if !strings.Contains(err.Error(), "Bad Request") || !strings.Contains(err.Error(), "Invalid organization name") {
        t.Errorf("expected JSON:API error details, got: %v", err)
    }
}
```

---

### 82. [ ] Missing test: verify correct org passed to API

**File:** `cmd/tfc/invoices_test.go`

**Problem:** Tests verify API methods are called and return expected results, but don't verify the correct organization is passed. After fixing issue #76, assertions should be added.

**Fix:** After updating `fakeInvoicesClient`, add assertions to tests:
```go
// In TestInvoicesList_JSON, after running cmd.Run():
fakeClient := &fakeInvoicesClient{listResponse: listResp}
cmd := &InvoicesListCmd{
    // ...
    clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
        return fakeClient, nil
    },
}
// Run test...
if fakeClient.listOrg != "test-org" {
    t.Errorf("expected org 'test-org', got %s", fakeClient.listOrg)
}
```

---

### 83. [ ] Missing test: output write failure

**File:** `cmd/tfc/invoices_test.go`

**Problem:** No test verifies error handling when writing to stdout fails. Lines 324-326 and 345-347 in `invoices.go` handle write errors, but this path is untested.

**Fix:** Add test with a writer that fails:
```go
type failingWriter struct{}

func (w *failingWriter) Write(p []byte) (int, error) {
    return 0, errors.New("write failed: disk full")
}

func TestInvoicesList_OutputWriteError(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    listResp := &InvoicesListResponse{
        Data: []InvoiceData{{ID: "inv-1", Type: "billing-invoices"}},
    }

    cmd := &InvoicesListCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &failingWriter{},
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return &fakeInvoicesClient{listResponse: listResp}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for write failure")
    }
    if !strings.Contains(err.Error(), "failed to write output") {
        t.Errorf("expected write error, got: %v", err)
    }
}
```

---

### 84. [ ] No integration test for `realInvoicesClient`

**File:** `cmd/tfc/invoices_test.go`

**Problem:** The `realInvoicesClient` struct and its HTTP handling logic are never directly tested. All tests inject `fakeInvoicesClient`. This means HTTP request construction, header setting, status code handling, and response parsing are untested.

**Fix:** Add integration-style tests using `httptest.Server`:
```go
func TestRealInvoicesClient_List_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify request
        if r.Method != "GET" {
            t.Errorf("expected GET, got %s", r.Method)
        }
        if r.URL.Path != "/api/v2/organizations/test-org/invoices" {
            t.Errorf("expected path /api/v2/organizations/test-org/invoices, got %s", r.URL.Path)
        }
        if r.Header.Get("Authorization") != "Bearer test-token" {
            t.Errorf("expected Bearer token, got %s", r.Header.Get("Authorization"))
        }
        if r.Header.Get("Content-Type") != "application/vnd.api+json" {
            t.Errorf("expected JSON:API content type, got %s", r.Header.Get("Content-Type"))
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(InvoicesListResponse{
            Data: []InvoiceData{
                {ID: "inv-123", Type: "billing-invoices"},
            },
        })
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    result, err := client.List(context.Background(), "test-org")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(result.Data) != 1 {
        t.Errorf("expected 1 invoice, got %d", len(result.Data))
    }
    if result.Data[0].ID != "inv-123" {
        t.Errorf("expected inv-123, got %s", result.Data[0].ID)
    }
}

func TestRealInvoicesClient_GetNext_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/v2/organizations/test-org/invoices/next" {
            t.Errorf("expected path /api/v2/organizations/test-org/invoices/next, got %s", r.URL.Path)
        }

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(InvoiceResponse{
            Data: InvoiceData{ID: "inv-next", Type: "billing-invoices"},
        })
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    result, err := client.GetNext(context.Background(), "test-org")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Data.ID != "inv-next" {
        t.Errorf("expected inv-next, got %s", result.Data.ID)
    }
}

func TestRealInvoicesClient_Unauthorized(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "bad-token",
        httpClient: server.Client(),
    }

    _, err := client.List(context.Background(), "test-org")
    if err == nil {
        t.Fatal("expected error for 401")
    }
    if !strings.Contains(err.Error(), "unauthorized") {
        t.Errorf("expected unauthorized error, got: %v", err)
    }
}

func TestRealInvoicesClient_Forbidden(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusForbidden)
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.List(context.Background(), "test-org")
    if err == nil {
        t.Fatal("expected error for 403")
    }
    if _, ok := err.(*invoicesNotAvailableError); !ok {
        t.Errorf("expected invoicesNotAvailableError, got: %T", err)
    }
}
```

---

### 85. [ ] Missing test: org with special characters

**File:** `cmd/tfc/invoices_test.go`

**Problem:** Lines 83 and 129 in `invoices.go` use `url.PathEscape(org)` to escape the organization name in the URL path. However, no test verifies this escaping works correctly for organization names with special characters.

**Fix:** Add test:
```go
func TestRealInvoicesClient_OrgWithSpecialChars(t *testing.T) {
    var requestedPath string
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestedPath = r.URL.Path
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(InvoicesListResponse{Data: []InvoiceData{}})
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    // Test with an org name that has characters needing escaping
    _, err := client.List(context.Background(), "org/with/slashes")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify slashes were escaped
    if requestedPath != "/api/v2/organizations/org%2Fwith%2Fslashes/invoices" {
        t.Errorf("expected escaped path, got %s", requestedPath)
    }
}
```

---

### 86. [ ] Missing test: malformed JSON response

**File:** `cmd/tfc/invoices_test.go`

**Problem:** No test verifies error handling when the API returns invalid JSON that cannot be parsed. Lines 112-114 and 155-157 in `invoices.go` handle this, but the path is untested.

**Fix:** Add test:
```go
func TestRealInvoicesClient_MalformedJSON(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{invalid json`))
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    _, err := client.List(context.Background(), "test-org")
    if err == nil {
        t.Fatal("expected error for malformed JSON")
    }
    if !strings.Contains(err.Error(), "failed to parse response") {
        t.Errorf("expected parse error, got: %v", err)
    }
}
```

---

### 87. [ ] Missing test: empty table output

**File:** `cmd/tfc/invoices_test.go`

**Problem:** `TestInvoicesList_EmptyList` only tests JSON output for empty list. No test verifies table output renders correctly with zero invoices.

**Fix:** Add test:
```go
func TestInvoicesList_EmptyList_Table(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    listResp := &InvoicesListResponse{Data: []InvoiceData{}}

    var stdout bytes.Buffer
    cmd := &InvoicesListCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return &fakeInvoicesClient{listResponse: listResp}, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := stdout.String()
    // Should still have headers
    if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") {
        t.Errorf("expected table headers even for empty list, got: %s", out)
    }
}
```

---

### 88. [ ] Missing test: InvoicesNext table output without ExternalLink

**File:** `cmd/tfc/invoices_test.go`

**Problem:** `TestInvoicesNext_Table` tests with ExternalLink populated. No test verifies table output when ExternalLink is empty (lines 432-434 in invoices.go have conditional logic).

**Fix:** Add test:
```go
func TestInvoicesNext_Table_NoExternalLink(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    nextResp := &InvoiceResponse{
        Data: InvoiceData{
            ID:   "inv-next123",
            Type: "billing-invoices",
            Attributes: InvoiceAttributes{
                CreatedAt:    time.Now(),
                Number:       "INV-001",
                Status:       "draft",
                Total:        5000,
                ExternalLink: "", // Empty
            },
        },
    }

    var stdout bytes.Buffer
    cmd := &InvoicesNextCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return &fakeInvoicesClient{nextResponse: nextResp}, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    out := stdout.String()
    // External Link row should not appear when empty
    if strings.Contains(out, "External Link") {
        t.Errorf("expected no External Link field when empty, got: %s", out)
    }
}
```

---

### 89. [ ] Missing test: InvoicesNext API error

**File:** `cmd/tfc/invoices_test.go`

**Problem:** `TestInvoicesList_APIError` tests API error for List, but there's no equivalent for `InvoicesNextCmd`.

**Fix:** Add test:
```go
func TestInvoicesNext_APIError(t *testing.T) {
    baseDir, tokenResolver := setupInvoicesTestSettings(t)

    var stdout bytes.Buffer
    cmd := &InvoicesNextCmd{
        baseDir:       baseDir,
        tokenResolver: tokenResolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        &stdout,
        clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
            return &fakeInvoicesClient{
                err: errors.New("Internal Server Error"),
            }, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error for API failure, got nil")
    }

    errStr := err.Error()
    if !strings.Contains(errStr, "Internal Server Error") {
        t.Errorf("expected 'Internal Server Error' in error, got: %s", errStr)
    }
}
```

---

### 90. [ ] `invoicesAPIError` test type doesn't match real error flow

**File:** `cmd/tfc/invoices_test.go`
**Lines:** 654-661

**Problem:** The `invoicesAPIError` type is defined for testing, but the error handling in `InvoicesListCmd.Run` (lines 303-313) specifically checks for `*invoicesNotAvailableError`, then uses `tfcapi.ParseAPIError`. The `invoicesAPIError` type doesn't interact with `tfcapi.ParseAPIError`, so the test at line 509-535 may not accurately simulate real error scenarios.

**Current:**
```go
type invoicesAPIError struct {
    message string
}

func (e *invoicesAPIError) Error() string {
    return e.message
}
```

**Fix:** Either remove the custom `invoicesAPIError` type and use `errors.New()` directly (which is what most error tests do), or ensure the test error type matches what the production code expects. For consistency with other test files:
```go
func TestInvoicesList_APIError(t *testing.T) {
    // ...
    clientFactory: func(_ tfcapi.ClientConfig) (invoicesClient, error) {
        return &fakeInvoicesClient{
            err: errors.New("Internal Server Error"),
        }, nil
    },
    // ...
}
```

Then delete the `invoicesAPIError` struct entirely.

---

### 91. [ ] `handleErrorResponse` doesn't include response body in generic error

**File:** `cmd/tfc/invoices.go`
**Line:** 199

**Problem:** When the HTTP status code is not one of the specifically handled codes (200, 401, 403, 404), and the JSON:API error parsing fails, the function returns a generic error with just the status code. The response body (which might contain useful debugging info) is discarded.

**Current:**
```go
return fmt.Errorf("API request failed with status %d", statusCode)
```

**Fix:** Include truncated body in error for debugging:
```go
bodyPreview := string(body)
if len(bodyPreview) > 200 {
    bodyPreview = bodyPreview[:200] + "..."
}
if len(bodyPreview) > 0 {
    return fmt.Errorf("API request failed with status %d: %s", statusCode, bodyPreview)
}
return fmt.Errorf("API request failed with status %d", statusCode)
```

---

### 92. [ ] Missing test: context cancellation during API call

**File:** `cmd/tfc/invoices_test.go`

**Problem:** No test verifies that the API calls correctly handle context cancellation. Lines 88 and 131 in `invoices.go` create requests with context, but cancellation isn't tested.

**Fix:** Add test:
```go
func TestRealInvoicesClient_ContextCancellation(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Simulate slow response
        time.Sleep(100 * time.Millisecond)
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(InvoicesListResponse{})
    }))
    defer server.Close()

    client := &realInvoicesClient{
        baseURL:    server.URL,
        token:      "test-token",
        httpClient: server.Client(),
    }

    ctx, cancel := context.WithCancel(context.Background())
    cancel() // Cancel immediately

    _, err := client.List(ctx, "test-org")
    if err == nil {
        t.Fatal("expected error for cancelled context")
    }
    if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "request") {
        t.Errorf("expected context/request error, got: %v", err)
    }
}
```
