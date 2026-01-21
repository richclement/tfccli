# Organizations Subcommand Code Review

Review of `cmd/tfc/organizations.go` and `cmd/tfc/organizations_test.go`.

---

## Finding 1: No-op updates allowed

**File:** `cmd/tfc/organizations.go:329-383`

**Issue:** `OrganizationsUpdateCmd` allows calling the API with no fields to update when `--email` is not provided. The command succeeds but does nothing, which may confuse users.

```go
opts := tfe.OrganizationUpdateOptions{}
if c.Email != "" {
    opts.Email = tfe.String(c.Email)
}
// API call happens even if opts is empty
org, err := client.Update(ctx, c.Name, opts)
```

**Fix:** Add validation before the API call to ensure at least one field is being updated. Return a user-friendly error if nothing to update.

```go
if c.Email == "" {
    return internalcmd.NewRuntimeError(errors.New("nothing to update: specify --email"))
}
```

---

## Finding 2: Empty organization list has no feedback

**File:** `cmd/tfc/organizations.go:164-172`

**Issue:** When listing organizations in table mode, an empty list renders only headers with no message indicating no organizations were found.

```go
tw := output.NewTableWriter(c.stdout, []string{"NAME", "EMAIL", "EXTERNAL-ID"}, isTTY)
for _, org := range orgs {  // If orgs is empty, loop never runs
    tw.AddRow(org.Name, org.Email, org.ExternalID)
}
```

**Fix:** Add a check after the API call to print a message when the list is empty (table mode only).

```go
if len(orgs) == 0 && format != output.FormatJSON {
    fmt.Fprintln(c.stdout, "No organizations found.")
    return nil
}
```

---

## Finding 3: Inconsistent error message formatting

**File:** `cmd/tfc/organizations.go:144-148` (and similar patterns throughout)

**Issue:** When `ParseAPIError` succeeds, the error uses `%s` which loses the error chain. When it fails, `%w` is used. This inconsistency prevents `errors.Is()` from working on API errors.

```go
if apiErr != nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %s", apiErr.Error()))
}
return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %w", err))
```

**Fix:** Use `%w` consistently to wrap errors, allowing error chain inspection.

```go
if apiErr != nil {
    return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %w", apiErr))
}
return internalcmd.NewRuntimeError(fmt.Errorf("failed to list organizations: %w", err))
```

Apply this pattern to all commands: List (line 146), Get (line 216), Create (line 292), Update (line 361), Delete (line 447).

---

## Finding 4: Missing test for OrganizationsGet table output

**File:** `cmd/tfc/organizations_test.go`

**Issue:** `TestOrganizationsGet_JSON` exists but there's no test for table output. The table path includes `CreatedAt` formatting which is untested.

**Fix:** Add a new test `TestOrganizationsGet_Table`:

```go
func TestOrganizationsGet_Table(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    fakeClient := &fakeOrgsClient{
        org: &tfe.Organization{
            Name:       "org-123",
            Email:      "admin@example.com",
            ExternalID: "ext-123",
            CreatedAt:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
        },
    }

    cmd := &OrganizationsGetCmd{
        Name:          "org-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    outStr := out.String()
    if !strings.Contains(outStr, "org-123") {
        t.Errorf("expected org name in output, got: %s", outStr)
    }
    if !strings.Contains(outStr, "admin@example.com") {
        t.Errorf("expected email in output, got: %s", outStr)
    }
}
```

---

## Finding 5: Missing test for OrganizationsUpdate no-op case

**File:** `cmd/tfc/organizations_test.go`

**Issue:** No test verifies behavior when `--email` is not provided. Currently the API is called with empty options.

**Fix:** Add a test that verifies the current behavior (or the fixed behavior after Finding 1 is addressed):

```go
func TestOrganizationsUpdate_NoFieldsProvided(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    fakeClient := &fakeOrgsClient{
        org: &tfe.Organization{Name: "org-123"},
    }

    cmd := &OrganizationsUpdateCmd{
        Name:          "org-123",
        Email:         "", // No email provided
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)

    // After fixing Finding 1, this should return an error
    if err == nil {
        t.Fatal("expected error when no fields provided")
    }
    if !strings.Contains(err.Error(), "nothing to update") {
        t.Errorf("expected 'nothing to update' error, got: %v", err)
    }
}
```

---

## Finding 6: Missing test for OrganizationsUpdate table output

**File:** `cmd/tfc/organizations_test.go`

**Issue:** Only `TestOrganizationsUpdate_JSON` exists. The table output path that prints `Organization %q updated.` is untested.

**Fix:** Add a new test:

```go
func TestOrganizationsUpdate_Table(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    fakeClient := &fakeOrgsClient{
        org: &tfe.Organization{
            Name:  "org-123",
            Email: "newemail@example.com",
        },
    }

    cmd := &OrganizationsUpdateCmd{
        Name:          "org-123",
        Email:         "newemail@example.com",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
            return fakeClient, nil
        },
    }

    cli := &CLI{OutputFormat: "table"}
    err := cmd.Run(cli)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if !strings.Contains(out.String(), "org-123") || !strings.Contains(out.String(), "updated") {
        t.Errorf("expected success message, got: %s", out.String())
    }
}
```

---

## Finding 7: Missing test for OrganizationsDelete table output

**File:** `cmd/tfc/organizations_test.go`

**Issue:** `TestOrganizationsDelete_JSON` tests JSON mode but there's no test for the table output message `Organization %q deleted.`.

**Fix:** Add a new test:

```go
func TestOrganizationsDelete_Table(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    fakeClient := &fakeOrgsClient{}
    forceFlag := true

    cmd := &OrganizationsDeleteCmd{
        Name:          "org-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
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

    if !strings.Contains(out.String(), "org-123") || !strings.Contains(out.String(), "deleted") {
        t.Errorf("expected delete message, got: %s", out.String())
    }
}
```

---

## Finding 8: Missing test for prompter error path

**File:** `cmd/tfc/organizations_test.go`

**Issue:** No test verifies behavior when the prompter returns an error. Line 423-425 in `organizations.go` handles this case but it's untested.

**Fix:** Create an `errorPrompter` and add a test:

```go
// errorPrompter returns an error for testing error paths.
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

func TestOrganizationsDelete_PrompterError(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    fakeClient := &fakeOrgsClient{}

    cmd := &OrganizationsDeleteCmd{
        Name:          "org-123",
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
            return fakeClient, nil
        },
        prompter: &errorPrompter{err: errors.New("terminal not available")},
    }

    cli := &CLI{Force: false}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to prompt") {
        t.Errorf("expected prompt error, got: %v", err)
    }
}
```

---

## Finding 9: Missing test for client factory error

**File:** `cmd/tfc/organizations_test.go`

**Issue:** No test verifies the error path when `clientFactory` returns an error (e.g., network issues creating the client).

**Fix:** Add a test for each command. Example for List:

```go
func TestOrganizationsList_ClientFactoryError(t *testing.T) {
    tmpDir, resolver := setupOrgsTestSettings(t)
    out := &bytes.Buffer{}

    cmd := &OrganizationsListCmd{
        baseDir:       tmpDir,
        tokenResolver: resolver,
        ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
        stdout:        out,
        clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
            return nil, errors.New("connection refused")
        },
    }

    cli := &CLI{OutputFormat: "json"}
    err := cmd.Run(cli)
    if err == nil {
        t.Fatal("expected error, got nil")
    }
    if !strings.Contains(err.Error(), "failed to create client") {
        t.Errorf("expected client creation error, got: %v", err)
    }
}
```

---

## Finding 10: forceFlag and cli.Force interaction undocumented

**File:** `cmd/tfc/organizations.go:415-418`

**Issue:** The `forceFlag` field allows tests to override `cli.Force`, but this interaction isn't documented. If both are set with conflicting values, `forceFlag` wins silently.

```go
force := cli.Force
if c.forceFlag != nil {
    force = *c.forceFlag
}
```

**Fix:** Add a comment explaining the precedence:

```go
// Get force flag from CLI or injected test value.
// Test injection (forceFlag) takes precedence over CLI flag.
force := cli.Force
if c.forceFlag != nil {
    force = *c.forceFlag
}
```

---

## Finding 11: Potential nil pointer with tfe.Organization fields

**File:** `cmd/tfc/organizations.go:167`

**Issue:** The code assumes `org.Email` and `org.ExternalID` are safe to access. While the go-tfe library returns empty strings for missing fields (not nil), this assumption isn't documented.

```go
tw.AddRow(org.Name, org.Email, org.ExternalID)
```

**Fix:** Add a defensive comment or handle potential empty strings explicitly for clarity:

```go
// Note: go-tfe returns empty strings for optional fields, not nil
tw.AddRow(org.Name, org.Email, org.ExternalID)
```

Alternatively, provide fallback display values:

```go
email := org.Email
if email == "" {
    email = "-"
}
externalID := org.ExternalID
if externalID == "" {
    externalID = "-"
}
tw.AddRow(org.Name, email, externalID)
```

---

## Summary

| # | Severity | Type | Fix Effort |
|---|----------|------|------------|
| 1 | Medium | Bug | Low |
| 2 | Low | UX | Low |
| 3 | Low | Code Quality | Low |
| 4 | Medium | Test Gap | Low |
| 5 | Medium | Test Gap | Low |
| 6 | Low | Test Gap | Low |
| 7 | Low | Test Gap | Low |
| 8 | Medium | Test Gap | Low |
| 9 | Medium | Test Gap | Low |
| 10 | Low | Documentation | Trivial |
| 11 | Low | Defensive Coding | Trivial |

**Recommended order:** 1, 3, 8, 9, 4, 5, 6, 7, 2, 10, 11

---

## Task Progress

### Finding 1: No-op updates allowed

**Status:** IN PROGRESS

**Acceptance Criteria:**
- `tfc organizations update <name>` without `--email` flag must return an error
- Error message must clearly state "nothing to update: specify --email"
- API must NOT be called if no fields are provided
- Test coverage for the no-email-provided path

**Verification:**
- Unit test `TestOrganizationsUpdate_NoFieldsProvided` must pass
- Existing tests must continue to pass
- `make lint && make test` green

**Implementation Plan:**
1. Add validation in `OrganizationsUpdateCmd.Run()` before the API call to check if `c.Email == ""`
2. Return a `RuntimeError` with message "nothing to update: specify --email"
3. Add test `TestOrganizationsUpdate_NoFieldsProvided` that expects error when no email is provided
4. Verify no API call is made (check `fakeClient.updateCalls` is empty)

**Progress Notes:**

**2026-01-21:** DONE

Files changed:
- `cmd/tfc/organizations.go`: Added validation before API call to check if `c.Email == ""`, returns error "nothing to update: specify --email". Simplified opts construction since email is now guaranteed.
- `cmd/tfc/organizations_test.go`: Added `TestOrganizationsUpdate_NoFieldsProvided` test that verifies:
  - Error is returned when no email is provided
  - Error message contains "nothing to update"
  - No API call is made (`fakeClient.updateCalls` is empty)

Commands run:
- `make fmt` - passed
- `make lint` - passed
- `make build` - passed
- `make test` - all tests pass
- `go test -v -run TestOrganizationsUpdate_NoFieldsProvided ./cmd/tfc/...` - passed

**Status:** DONE
