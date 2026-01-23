package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeWorkspacesClient is a test double for workspacesClient.
type fakeWorkspacesClient struct {
	workspaces []*tfe.Workspace
	workspace  *tfe.Workspace
	listErr    error
	readErr    error
	createErr  error
	updateErr  error
	deleteErr  error

	// Track calls for assertions
	listCalls []struct {
		org  string
		opts *tfe.WorkspaceListOptions
	}
	readCalls   []string
	createCalls []struct {
		org  string
		opts tfe.WorkspaceCreateOptions
	}
	updateCalls []struct {
		id   string
		opts tfe.WorkspaceUpdateOptions
	}
	deleteCalls []string
}

func (f *fakeWorkspacesClient) List(_ context.Context, org string, opts *tfe.WorkspaceListOptions) ([]*tfe.Workspace, error) {
	f.listCalls = append(f.listCalls, struct {
		org  string
		opts *tfe.WorkspaceListOptions
	}{org, opts})
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.workspaces, nil
}

func (f *fakeWorkspacesClient) ReadByID(_ context.Context, workspaceID string) (*tfe.Workspace, error) {
	f.readCalls = append(f.readCalls, workspaceID)
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.workspace, nil
}

func (f *fakeWorkspacesClient) Create(_ context.Context, org string, opts tfe.WorkspaceCreateOptions) (*tfe.Workspace, error) {
	f.createCalls = append(f.createCalls, struct {
		org  string
		opts tfe.WorkspaceCreateOptions
	}{org, opts})
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.workspace, nil
}

func (f *fakeWorkspacesClient) UpdateByID(_ context.Context, workspaceID string, opts tfe.WorkspaceUpdateOptions) (*tfe.Workspace, error) {
	f.updateCalls = append(f.updateCalls, struct {
		id   string
		opts tfe.WorkspaceUpdateOptions
	}{workspaceID, opts})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.workspace, nil
}

func (f *fakeWorkspacesClient) DeleteByID(_ context.Context, workspaceID string) error {
	f.deleteCalls = append(f.deleteCalls, workspaceID)
	return f.deleteErr
}

// setupWorkspacesTestSettings creates test settings with default_org and returns the temp directory and token resolver.
func setupWorkspacesTestSettings(t *testing.T, defaultOrg string) (string, *auth.TokenResolver) {
	t.Helper()
	tmpDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    "app.terraform.io",
				DefaultOrg: defaultOrg,
				LogLevel:   "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save test settings: %v", err)
	}

	// Create fake env with token
	fakeEnvMap := &testEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFSMap := &testFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap}
	return tmpDir, resolver
}

// TestWorkspacesList_UsesDefaultOrg tests that list uses default_org when --org not provided.
func TestWorkspacesList_UsesDefaultOrg(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{
			{ID: "ws-1", Name: "workspace-1", ExecutionMode: "remote"},
		},
	}

	cmd := &WorkspacesListCmd{
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

	// Verify the list was called with "acme" org
	if len(fakeClient.listCalls) != 1 {
		t.Errorf("expected 1 list call, got %d", len(fakeClient.listCalls))
	}
	if fakeClient.listCalls[0].org != "acme" {
		t.Errorf("expected org acme, got %s", fakeClient.listCalls[0].org)
	}
}

// TestWorkspacesList_UsesOrgFlag tests that --org flag overrides default_org.
func TestWorkspacesList_UsesOrgFlag(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "default-org")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{},
	}

	cmd := &WorkspacesListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{Org: "override-org", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the list was called with override org
	if len(fakeClient.listCalls) != 1 {
		t.Errorf("expected 1 list call, got %d", len(fakeClient.listCalls))
	}
	if fakeClient.listCalls[0].org != "override-org" {
		t.Errorf("expected org override-org, got %s", fakeClient.listCalls[0].org)
	}
}

// TestWorkspacesList_FailsWhenNoOrg tests that list fails when no org is available.
func TestWorkspacesList_FailsWhenNoOrg(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "") // empty default_org
	out := &bytes.Buffer{}

	cmd := &WorkspacesListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return &fakeWorkspacesClient{}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "organization is required") {
		t.Errorf("expected 'organization is required' error, got: %v", err)
	}
	// Verify it's NOT a RuntimeError - should be exit code 1 (usage error per PRD)
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		t.Errorf("expected plain error (exit code 1), got RuntimeError (exit code 2)")
	}
}

// TestWorkspacesList_JSON tests that list returns workspaces as JSON.
func TestWorkspacesList_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{
			{ID: "ws-1", Name: "workspace-1", ExecutionMode: "remote"},
			{ID: "ws-2", Name: "workspace-2", ExecutionMode: "local"},
		},
	}

	cmd := &WorkspacesListCmd{
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

	// Parse JSON output
	var result struct {
		Data []workspaceJSON `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(result.Data))
	}
}

// TestWorkspacesList_Table tests that list returns workspaces as table.
func TestWorkspacesList_Table(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{
			{ID: "ws-1", Name: "workspace-1", ExecutionMode: "remote"},
		},
	}

	cmd := &WorkspacesListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "ID") || !strings.Contains(outStr, "NAME") || !strings.Contains(outStr, "EXECUTION-MODE") {
		t.Errorf("expected table headers, got: %s", outStr)
	}
	if !strings.Contains(outStr, "ws-1") || !strings.Contains(outStr, "workspace-1") {
		t.Errorf("expected workspace data in output, got: %s", outStr)
	}
}

// TestWorkspacesList_WithProjectFilter tests that list passes project filter to API.
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

// TestWorkspacesList_WithSearch tests that list passes search filter to API.
func TestWorkspacesList_WithSearch(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{
			{ID: "ws-1", Name: "my-workspace", ExecutionMode: "remote"},
		},
	}

	cmd := &WorkspacesListCmd{
		Search:        "my-",
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

	// Verify the list was called with search filter
	if len(fakeClient.listCalls) != 1 {
		t.Errorf("expected 1 list call, got %d", len(fakeClient.listCalls))
	}
	if fakeClient.listCalls[0].opts == nil || fakeClient.listCalls[0].opts.Search != "my-" {
		t.Errorf("expected search filter 'my-', got: %+v", fakeClient.listCalls[0].opts)
	}
}

// TestWorkspacesList_WithTags tests that list passes tags filter to API.
func TestWorkspacesList_WithTags(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspaces: []*tfe.Workspace{
			{ID: "ws-1", Name: "tagged-workspace", ExecutionMode: "remote"},
		},
	}

	cmd := &WorkspacesListCmd{
		Tags:          "env:prod,team:platform",
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

	// Verify the list was called with tags filter
	if len(fakeClient.listCalls) != 1 {
		t.Errorf("expected 1 list call, got %d", len(fakeClient.listCalls))
	}
	if fakeClient.listCalls[0].opts == nil || fakeClient.listCalls[0].opts.Tags != "env:prod,team:platform" {
		t.Errorf("expected tags filter 'env:prod,team:platform', got: %+v", fakeClient.listCalls[0].opts)
	}
}

// TestWorkspacesGet_JSON tests getting a workspace by ID.
func TestWorkspacesGet_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-123",
			Name:          "test-workspace",
			Description:   "Test description",
			ExecutionMode: "remote",
		},
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.readCalls) != 1 || fakeClient.readCalls[0] != "ws-123" {
		t.Errorf("expected read call for ws-123, got: %v", fakeClient.readCalls)
	}

	// Parse JSON output
	var result struct {
		Data *workspaceJSON `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if result.Data.ID != "ws-123" {
		t.Errorf("expected ID ws-123, got %s", result.Data.ID)
	}
}

// TestWorkspacesCreate_JSON tests creating a workspace.
func TestWorkspacesCreate_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-new",
			Name:          "new-workspace",
			Description:   "New workspace description",
			ExecutionMode: "remote",
		},
	}

	cmd := &WorkspacesCreateCmd{
		Name:          "new-workspace",
		Description:   "New workspace description",
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

	if len(fakeClient.createCalls) != 1 {
		t.Errorf("expected 1 create call, got %d", len(fakeClient.createCalls))
	}
	if fakeClient.createCalls[0].org != "acme" {
		t.Errorf("expected org acme, got %s", fakeClient.createCalls[0].org)
	}
	if *fakeClient.createCalls[0].opts.Name != "new-workspace" {
		t.Errorf("expected name new-workspace, got %s", *fakeClient.createCalls[0].opts.Name)
	}
}

// TestWorkspacesCreate_WithProjectID tests creating a workspace with --project-id.
func TestWorkspacesCreate_WithProjectID(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-new",
			Name:          "new-workspace",
			ExecutionMode: "remote",
			Project:       &tfe.Project{ID: "prj-123"},
		},
	}

	cmd := &WorkspacesCreateCmd{
		Name:          "new-workspace",
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

	if len(fakeClient.createCalls) != 1 {
		t.Errorf("expected 1 create call, got %d", len(fakeClient.createCalls))
	}
	if fakeClient.createCalls[0].opts.Project == nil || fakeClient.createCalls[0].opts.Project.ID != "prj-123" {
		t.Errorf("expected project ID prj-123 in create options")
	}
}

// TestWorkspacesCreate_FailsWhenNoOrg tests that create fails when no org available.
func TestWorkspacesCreate_FailsWhenNoOrg(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "") // empty default_org
	out := &bytes.Buffer{}

	cmd := &WorkspacesCreateCmd{
		Name:          "new-workspace",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return &fakeWorkspacesClient{}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "organization is required") {
		t.Errorf("expected 'organization is required' error, got: %v", err)
	}
	// Verify it's NOT a RuntimeError - should be exit code 1 (usage error per PRD)
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		t.Errorf("expected plain error (exit code 1), got RuntimeError (exit code 2)")
	}
}

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
	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}
}

// TestWorkspacesUpdate_JSON tests updating a workspace.
func TestWorkspacesUpdate_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-123",
			Name:          "updated-workspace",
			Description:   "Updated description",
			ExecutionMode: "remote",
		},
	}

	cmd := &WorkspacesUpdateCmd{
		ID:            "ws-123",
		Name:          "updated-workspace",
		Description:   "Updated description",
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

	if len(fakeClient.updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(fakeClient.updateCalls))
	}
	if fakeClient.updateCalls[0].id != "ws-123" {
		t.Errorf("expected id ws-123, got %s", fakeClient.updateCalls[0].id)
	}
	if *fakeClient.updateCalls[0].opts.Name != "updated-workspace" {
		t.Errorf("expected name updated-workspace, got %s", *fakeClient.updateCalls[0].opts.Name)
	}
}

// TestWorkspacesUpdate_FailsWhenNoFields tests that update fails when no fields provided.
func TestWorkspacesUpdate_FailsWhenNoFields(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{}

	cmd := &WorkspacesUpdateCmd{
		ID: "ws-123",
		// No Name or Description provided
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
		t.Fatal("expected error when no fields provided, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	// Verify error message
	if !strings.Contains(err.Error(), "at least one of --name, --description, or --clear-description is required") {
		t.Errorf("expected 'at least one of' error message, got: %v", err)
	}

	// Verify no API call was made
	if len(fakeClient.updateCalls) != 0 {
		t.Errorf("expected no update calls, got %d", len(fakeClient.updateCalls))
	}
}

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
	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}
}

// TestWorkspacesDelete_PromptsWithoutForce tests that delete prompts without --force.
func TestWorkspacesDelete_PromptsWithoutForce(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
		prompter: &rejectingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not have called delete
	if len(fakeClient.deleteCalls) != 0 {
		t.Errorf("expected no delete calls when user says no, got: %v", fakeClient.deleteCalls)
	}

	if !strings.Contains(out.String(), "Aborting") {
		t.Errorf("expected abort message, got: %s", out.String())
	}
}

// TestWorkspacesDelete_WithForce tests that delete bypasses prompt with --force.
func TestWorkspacesDelete_WithForce(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
		prompter: &failingPrompter{}, // Would fail if called
	}

	cli := &CLI{Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "ws-123" {
		t.Errorf("expected delete call for ws-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestWorkspacesDelete_JSON tests that delete returns proper JSON on success.
func TestWorkspacesDelete_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
		prompter: &failingPrompter{},
	}

	cli := &CLI{OutputFormat: "json", Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse JSON output
	var result struct {
		Meta struct {
			Status int `json:"status"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if result.Meta.Status != 204 {
		t.Errorf("expected status 204, got %d", result.Meta.Status)
	}
}

// TestWorkspacesList_APIError tests that API errors are surfaced.
func TestWorkspacesList_APIError(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		listErr: errors.New("unauthorized"),
	}

	cmd := &WorkspacesListCmd{
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
	if !strings.Contains(err.Error(), "failed to list workspaces") {
		t.Errorf("expected error message about list failure, got: %v", err)
	}
}

// TestWorkspacesGet_NotFound tests 404 error handling.
func TestWorkspacesGet_NotFound(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		readErr: tfe.ErrResourceNotFound,
	}

	cmd := &WorkspacesGetCmd{
		ID:            "nonexistent",
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
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("expected Not Found error, got: %v", err)
	}
}

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
		t.Errorf("expected 'failed to get workspace' error, got: %v", err)
	}
	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}
}

// TestWorkspacesList_FailsWhenSettingsMissing tests error when settings don't exist.
func TestWorkspacesList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir() // Empty dir, no settings
	out := &bytes.Buffer{}

	cmd := &WorkspacesListCmd{
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      out,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tfc init") {
		t.Errorf("expected error suggesting tfc init, got: %v", err)
	}
}

// TestWorkspacesDelete_ConfirmYes tests that delete proceeds when user confirms.
func TestWorkspacesDelete_ConfirmYes(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
		prompter: &acceptingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "ws-123" {
		t.Errorf("expected delete call for ws-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestWorkspacesDelete_PrompterError tests that prompter errors are surfaced.
func TestWorkspacesDelete_PrompterError(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return &fakeWorkspacesClient{}, nil
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
	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}
}

// TestWorkspacesDelete_APIError tests that API errors are surfaced during delete.
func TestWorkspacesDelete_APIError(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		deleteErr: errors.New("workspace has active runs"),
	}

	cmd := &WorkspacesDeleteCmd{
		ID:            "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
		prompter: &failingPrompter{},
	}

	cli := &CLI{Force: true}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete workspace") {
		t.Errorf("expected delete failure message, got: %v", err)
	}
	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}
}

// TestWorkspacesCreate_Table tests that create outputs a success message in table mode.
func TestWorkspacesCreate_Table(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-new",
			Name:          "new-workspace",
			ExecutionMode: "remote",
		},
	}

	cmd := &WorkspacesCreateCmd{
		Name:          "new-workspace",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out.String(), "new-workspace") || !strings.Contains(out.String(), "created") {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

// TestWorkspacesUpdate_ClearDescription tests that --clear-description sends empty string to API.
func TestWorkspacesUpdate_ClearDescription(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeWorkspacesClient{
		workspace: &tfe.Workspace{
			ID:            "ws-123",
			Name:          "test-workspace",
			Description:   "", // Cleared
			ExecutionMode: "remote",
		},
	}

	cmd := &WorkspacesUpdateCmd{
		ID:               "ws-123",
		ClearDescription: true,
		baseDir:          tmpDir,
		tokenResolver:    resolver,
		ttyDetector:      &output.FakeTTYDetector{IsTTYValue: false},
		stdout:           out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the update was called with empty description
	if len(fakeClient.updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(fakeClient.updateCalls))
	}
	if fakeClient.updateCalls[0].opts.Description == nil {
		t.Fatal("expected Description to be set in update options")
	}
	if *fakeClient.updateCalls[0].opts.Description != "" {
		t.Errorf("expected empty description, got %q", *fakeClient.updateCalls[0].opts.Description)
	}
}

// TestWorkspacesUpdate_ClearDescriptionConflict tests that --description and --clear-description are mutually exclusive.
func TestWorkspacesUpdate_ClearDescriptionConflict(t *testing.T) {
	tmpDir, resolver := setupWorkspacesTestSettings(t, "acme")
	out := &bytes.Buffer{}

	cmd := &WorkspacesUpdateCmd{
		ID:               "ws-123",
		Description:      "new description",
		ClearDescription: true, // Conflict!
		baseDir:          tmpDir,
		tokenResolver:    resolver,
		ttyDetector:      &output.FakeTTYDetector{IsTTYValue: false},
		stdout:           out,
		clientFactory: func(_ tfcapi.ClientConfig) (workspacesClient, error) {
			return &fakeWorkspacesClient{}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when both --description and --clear-description provided, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	// Verify error message mentions mutual exclusivity
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error message, got: %v", err)
	}
}
