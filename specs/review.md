# Projects Subcommand Code Review - Implementation Tasks

This document contains issues identified during code review of the `projects` subcommand. Tasks are organized by priority.

---

## High Priority

### 1. Add validation for empty update options

**Status:** DONE

**File:** `cmd/tfc/projects.go`
**Lines:** 401-407

**Problem:**
The `ProjectsUpdateCmd` allows running `tfc projects update prj-123` without specifying `--name` or `--description`. This makes an API call with empty options, which is a no-op.

**Plan:**
- Acceptance criteria: Running `tfc projects update prj-123` (no --name or --description) returns an error with exit code 2 (RuntimeError) and message "at least one of --name or --description is required"
- Verification: Add unit test `TestProjectsUpdate_FailsWhenNoChanges`
- Implementation:
  1. Add validation check in `ProjectsUpdateCmd.Run` after defaults setup, before client config resolution
  2. Return `internalcmd.NewRuntimeError` if both Name and Description are empty
  3. Add test to verify error message and that no API calls are made

**Progress (2026-01-21):**
- Added validation check in `cmd/tfc/projects.go:390-393`
- Added test `TestProjectsUpdate_FailsWhenNoChanges` in `cmd/tfc/projects_test.go`
- Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Verified: error returns RuntimeError (exit code 2), correct message, no API calls made

**Current code:**
```go
opts := tfe.ProjectUpdateOptions{}
if c.Name != "" {
    opts.Name = &c.Name
}
if c.Description != "" {
    opts.Description = &c.Description
}
```

**Fix:**
Add validation after line 400, before building opts:

```go
// Validate at least one field is being updated
if c.Name == "" && c.Description == "" {
    return fmt.Errorf("at least one of --name or --description is required")
}
```

**Test to add in `cmd/tfc/projects_test.go`:**
```go
func TestProjectsUpdate_FailsWhenNoChanges(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    cmd := &ProjectsUpdateCmd{
        ID:            "prj-123",
        // Name and Description both empty
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return &fakeProjectsClient{}, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "at least one of --name or --description") {
        t.Errorf("expected validation error, got: %v", err)
    }
}
```

---

### 2. Wrap "organization required" errors with RuntimeError

**Status:** DONE

**Files:** `cmd/tfc/projects.go`
**Lines:** 171, 319

**Problem:**
The "organization is required" errors are not wrapped with `NewRuntimeError`, causing exit code 1 (usage error) instead of exit code 2 (runtime error). This is inconsistent with other runtime errors in the same functions.

**Plan:**
- Acceptance criteria: `tfc projects list` and `tfc projects create` return exit code 2 (RuntimeError) when org is missing (no default_org in context and no --org flag)
- Verification: Add type assertions to existing tests `TestProjectsList_FailsWhenNoOrg` and `TestProjectsCreate_FailsWhenNoOrg`
- Implementation:
  1. Wrap error at line 171 with `internalcmd.NewRuntimeError`
  2. Wrap error at line 319 with `internalcmd.NewRuntimeError`
  3. Add `errors.As` assertions in tests to verify RuntimeError type

**Current code (line 171):**
```go
return fmt.Errorf("organization is required; use --org flag or set default_org in context")
```

**Fix:**
Wrap with `internalcmd.NewRuntimeError`:

```go
return internalcmd.NewRuntimeError(fmt.Errorf("organization is required; use --org flag or set default_org in context"))
```

**Apply the same fix to line 319** in `ProjectsCreateCmd.Run`.

**No new test needed** - existing tests `TestProjectsList_FailsWhenNoOrg` and `TestProjectsCreate_FailsWhenNoOrg` will verify the error message. Optionally add assertions checking the error type:

```go
var runtimeErr *internalcmd.RuntimeError
if !errors.As(err, &runtimeErr) {
    t.Errorf("expected RuntimeError, got %T", err)
}
```

**Progress (2026-01-21):**
- Wrapped errors at lines 171 and 319 in `cmd/tfc/projects.go` with `internalcmd.NewRuntimeError`
- Added `internalcmd` import to `cmd/tfc/projects_test.go`
- Added RuntimeError type assertions to both `TestProjectsList_FailsWhenNoOrg` and `TestProjectsCreate_FailsWhenNoOrg`
- Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Specific tests verified: `go test -v -run "TestProjectsList_FailsWhenNoOrg|TestProjectsCreate_FailsWhenNoOrg" ./cmd/tfc/` - pass

---

## Medium Priority

### 3. Add missing table output tests

**Status:** DONE

**File:** `cmd/tfc/projects_test.go`

**Problem:**
Table output format is not tested for `Get`, `Update`, and `Delete` commands.

**Plan:**
- Acceptance criteria: Table output tests exist for Get, Update, and Delete commands verifying human-readable output
- Verification: `go test -v -run "TestProjectsGet_Table|TestProjectsUpdate_Table|TestProjectsDelete_Table" ./cmd/tfc/`
- Implementation:
  1. Add `TestProjectsGet_Table` - verify FIELD/VALUE headers and project data
  2. Add `TestProjectsUpdate_Table` - verify updated project info in output
  3. Add `TestProjectsDelete_Table` - verify "deleted" message

**Progress (2026-01-21):**
- Added `TestProjectsGet_Table` (lines 744-774): verifies FIELD/VALUE table headers and project data
- Added `TestProjectsUpdate_Table` (lines 776-808): verifies "updated" message with project name
- Added `TestProjectsDelete_Table` (lines 810-842): verifies "deleted" message with project ID
- Commands run: `make fmt`, `make lint`, `make build`, `make test` - all pass
- Specific tests verified: `go test -v -run "TestProjectsGet_Table|TestProjectsUpdate_Table|TestProjectsDelete_Table" ./cmd/tfc/` - all pass

**Tests to add:**

```go
func TestProjectsGet_Table(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{
        project: &tfe.Project{
            ID:          "prj-123",
            Name:        "test-project",
            Description: "Test description",
        },
    }

    cmd := &ProjectsGetCmd{
        ID:            "prj-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    outStr := out.String()
    if !strings.Contains(outStr, "FIELD") || !strings.Contains(outStr, "VALUE") {
        t.Errorf("expected table headers, got: %s", outStr)
    }
    if !strings.Contains(outStr, "prj-123") || !strings.Contains(outStr, "test-project") {
        t.Errorf("expected project data in output, got: %s", outStr)
    }
}

func TestProjectsUpdate_Table(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{
        project: &tfe.Project{
            ID:   "prj-123",
            Name: "updated-project",
        },
    }

    cmd := &ProjectsUpdateCmd{
        ID:            "prj-123",
        Name:          "updated-project",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if !strings.Contains(out.String(), "updated-project") || !strings.Contains(out.String(), "updated") {
        t.Errorf("expected success message, got: %s", out.String())
    }
}

func TestProjectsDelete_Table(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{}
    forceFlag := true

    cmd := &ProjectsDeleteCmd{
        ID:            "prj-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
        prompter:  &failingPrompter{},
        forceFlag: &forceFlag,
    }

    cli := &CLI{OutputFormat: "table", Force: true}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if !strings.Contains(out.String(), "prj-123") || !strings.Contains(out.String(), "deleted") {
        t.Errorf("expected success message, got: %s", out.String())
    }
}
```

---

### 4. Add API error tests for create/update/delete

**File:** `cmd/tfc/projects_test.go`

**Problem:**
API error paths are not tested for `Create`, `Update`, and `Delete` commands.

**Tests to add:**

```go
func TestProjectsCreate_APIError(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{
        createErr: errors.New("name already exists"),
    }

    cmd := &ProjectsCreateCmd{
        Name:          "existing-project",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to create project") {
        t.Errorf("expected error message about create failure, got: %v", err)
    }
}

func TestProjectsUpdate_APIError(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{
        updateErr: errors.New("project not found"),
    }

    cmd := &ProjectsUpdateCmd{
        ID:            "prj-nonexistent",
        Name:          "new-name",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to update project") {
        t.Errorf("expected error message about update failure, got: %v", err)
    }
}

func TestProjectsDelete_APIError(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{
        deleteErr: errors.New("cannot delete project with workspaces"),
    }
    forceFlag := true

    cmd := &ProjectsDeleteCmd{
        ID:            "prj-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
        prompter:  &failingPrompter{},
        forceFlag: &forceFlag,
    }

    cli := &CLI{OutputFormat: "json", Force: true}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to delete project") {
        t.Errorf("expected error message about delete failure, got: %v", err)
    }
}
```

---

### 5. Add prompter error test for delete

**File:** `cmd/tfc/projects_test.go`

**Problem:**
The error path when the prompter returns an error is not tested.

**Test to add:**

```go
func TestProjectsDelete_PromptError(t *testing.T) {
    tmpDir, resolver := setupProjectsTestSettings(t, "acme")
    out := &bytes.Buffer{}

    fakeClient := &fakeProjectsClient{}

    cmd := &ProjectsDeleteCmd{
        ID:            "prj-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
            return fakeClient, nil
        },
        prompter: &errorPrompter{err: errors.New("terminal not available")},
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatalf("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
        t.Errorf("expected prompt error, got: %v", err)
    }

    // Should not have called delete
    if len(fakeClient.deleteCalls) != 0 {
        t.Errorf("expected no delete calls on prompt error, got: %v", fakeClient.deleteCalls)
    }
}
```

**Note:** This test uses `errorPrompter` which is already defined in `organizations_test.go`.

---

## Low Priority

### 6. Extract TTY detection to helper function

**File:** `cmd/tfc/projects.go`
**Lines:** 190-194, 261-265, 345-349, 419-423, 506-510

**Problem:**
The same TTY detection boilerplate is repeated 5 times across the file.

**Current code (repeated):**
```go
isTTY := false
if f, ok := c.stdout.(*os.File); ok {
    isTTY = c.ttyDetector.IsTTY(f)
}
format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)
```

**Fix:**
Create a helper function. Add near the top of the file after the type definitions:

```go
// resolveFormat determines the output format based on CLI flags and TTY detection.
func resolveFormat(stdout io.Writer, ttyDetector output.TTYDetector, cliFormat string) output.Format {
    isTTY := false
    if f, ok := stdout.(*os.File); ok {
        isTTY = ttyDetector.IsTTY(f)
    }
    return output.ResolveOutputFormat(cliFormat, isTTY)
}
```

Then replace each occurrence with:
```go
format := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)
```

**Note:** This is a refactoring task. Run `make test` after to verify no regressions.

---

### 7. Move test prompters to shared file

**Files:**
- `cmd/tfc/organizations_test.go` (source)
- `cmd/tfc/projects_test.go` (consumer)

**Problem:**
Test helper types (`acceptingPrompter`, `rejectingPrompter`, `failingPrompter`, `errorPrompter`) are defined in `organizations_test.go` but used by `projects_test.go`. This creates implicit coupling.

**Fix:**
Create a new file `cmd/tfc/testhelpers_test.go` and move the prompter definitions there:

```go
package main

import "errors"

// acceptingPrompter always returns true for confirms.
type acceptingPrompter struct{}

func (p *acceptingPrompter) PromptString(_, defaultValue string) (string, error) {
    return defaultValue, nil
}

func (p *acceptingPrompter) Confirm(_ string, _ bool) (bool, error) {
    return true, nil
}

func (p *acceptingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
    return defaultValue, nil
}

// rejectingPrompter always returns false for confirms.
type rejectingPrompter struct{}

func (p *rejectingPrompter) PromptString(_, defaultValue string) (string, error) {
    return defaultValue, nil
}

func (p *rejectingPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, nil
}

func (p *rejectingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
    return defaultValue, nil
}

// failingPrompter returns an error to verify prompts are bypassed with --force.
type failingPrompter struct{}

func (p *failingPrompter) PromptString(_, _ string) (string, error) {
    return "", errors.New("should not be called with --force")
}

func (p *failingPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, errors.New("should not be called with --force")
}

func (p *failingPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", errors.New("should not be called with --force")
}

// errorPrompter returns a configurable error for testing prompter error paths.
type errorPrompter struct {
    err error
}

func (p *errorPrompter) PromptString(_, _ string) (string, error) {
    return "", p.err
}

func (p *errorPrompter) Confirm(_ string, _ bool) (bool, error) {
    return false, p.err
}

func (p *errorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
    return "", p.err
}
```

Then remove these definitions from `organizations_test.go`.

**Note:** Run `make test` after to verify no regressions.

---

## Verification

After implementing all fixes, run:

```bash
make ci
```

This runs `fmt-check`, `lint`, and `test` to ensure all changes are correct.
