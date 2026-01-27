package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeProjectsClient is a test double for projectsClient.
type fakeProjectsClient struct {
	projects  []*tfe.Project
	project   *tfe.Project
	listErr   error
	readErr   error
	createErr error
	updateErr error
	deleteErr error

	// Track calls for assertions
	listCalls []struct {
		org  string
		opts *tfe.ProjectListOptions
	}
	readCalls   []string
	createCalls []struct {
		org  string
		opts tfe.ProjectCreateOptions
	}
	updateCalls []struct {
		id   string
		opts tfe.ProjectUpdateOptions
	}
	deleteCalls []string
}

func (f *fakeProjectsClient) List(_ context.Context, org string, opts *tfe.ProjectListOptions) ([]*tfe.Project, error) {
	f.listCalls = append(f.listCalls, struct {
		org  string
		opts *tfe.ProjectListOptions
	}{org, opts})
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.projects, nil
}

func (f *fakeProjectsClient) Read(_ context.Context, projectID string) (*tfe.Project, error) {
	f.readCalls = append(f.readCalls, projectID)
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.project, nil
}

func (f *fakeProjectsClient) Create(_ context.Context, org string, opts tfe.ProjectCreateOptions) (*tfe.Project, error) {
	f.createCalls = append(f.createCalls, struct {
		org  string
		opts tfe.ProjectCreateOptions
	}{org, opts})
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.project, nil
}

func (f *fakeProjectsClient) Update(_ context.Context, projectID string, opts tfe.ProjectUpdateOptions) (*tfe.Project, error) {
	f.updateCalls = append(f.updateCalls, struct {
		id   string
		opts tfe.ProjectUpdateOptions
	}{projectID, opts})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.project, nil
}

func (f *fakeProjectsClient) Delete(_ context.Context, projectID string) error {
	f.deleteCalls = append(f.deleteCalls, projectID)
	return f.deleteErr
}

// projectsTestEnv implements auth.EnvGetter for testing.
type projectsTestEnv struct {
	vars map[string]string
}

func (e *projectsTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// projectsTestFS implements auth.FSReader for testing.
type projectsTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *projectsTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *projectsTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// setupProjectsTestSettings creates test settings with default_org and returns the temp directory and token resolver.
func setupProjectsTestSettings(t *testing.T, defaultOrg string) (string, *auth.TokenResolver) {
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
	fakeEnvMap := &projectsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFSMap := &projectsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap}
	return tmpDir, resolver
}

// TestProjectsList_UsesDefaultOrg tests that list uses default_org when --org not provided.
func TestProjectsList_UsesDefaultOrg(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		projects: []*tfe.Project{
			{ID: "prj-1", Name: "project-1", Description: "First project"},
		},
	}

	cmd := &ProjectsListCmd{
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

// TestProjectsList_UsesOrgFlag tests that --org flag overrides default_org.
func TestProjectsList_UsesOrgFlag(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "default-org")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		projects: []*tfe.Project{},
	}

	cmd := &ProjectsListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
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

// TestProjectsList_FailsWhenNoOrg tests that list fails when no org is available.
func TestProjectsList_FailsWhenNoOrg(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "") // empty default_org
	out := &bytes.Buffer{}

	cmd := &ProjectsListCmd{
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
	if !strings.Contains(err.Error(), "organization is required") {
		t.Errorf("expected 'organization is required' error, got: %v", err)
	}
	// Verify it's NOT a RuntimeError - should be exit code 1 (usage error per PRD)
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		t.Errorf("expected plain error (exit code 1), got RuntimeError (exit code 2)")
	}
}

// TestProjectsList_JSON tests that list returns projects as JSON.
func TestProjectsList_JSON(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		projects: []*tfe.Project{
			{ID: "prj-1", Name: "project-1", Description: "First project"},
			{ID: "prj-2", Name: "project-2", Description: "Second project"},
		},
	}

	cmd := &ProjectsListCmd{
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse JSON output
	var result struct {
		Data []projectJSON `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 projects, got %d", len(result.Data))
	}
}

// TestProjectsList_Table tests that list returns projects as table.
func TestProjectsList_Table(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		projects: []*tfe.Project{
			{ID: "prj-1", Name: "project-1", Description: "First project"},
		},
	}

	cmd := &ProjectsListCmd{
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
	if !strings.Contains(outStr, "ID") || !strings.Contains(outStr, "NAME") || !strings.Contains(outStr, "DESCRIPTION") {
		t.Errorf("expected table headers, got: %s", outStr)
	}
	if !strings.Contains(outStr, "prj-1") || !strings.Contains(outStr, "project-1") {
		t.Errorf("expected project data in output, got: %s", outStr)
	}
}

// TestProjectsGet_JSON tests getting a project by ID.
func TestProjectsGet_JSON(t *testing.T) {
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.readCalls) != 1 || fakeClient.readCalls[0] != "prj-123" {
		t.Errorf("expected read call for prj-123, got: %v", fakeClient.readCalls)
	}

	// Parse JSON output
	var result struct {
		Data *projectJSON `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if result.Data.ID != "prj-123" {
		t.Errorf("expected ID prj-123, got %s", result.Data.ID)
	}
}

// TestProjectsCreate_JSON tests creating a project.
func TestProjectsCreate_JSON(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		project: &tfe.Project{
			ID:          "prj-new",
			Name:        "new-project",
			Description: "New project description",
		},
	}

	cmd := &ProjectsCreateCmd{
		Name:          "new-project",
		Description:   "New project description",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.createCalls) != 1 {
		t.Errorf("expected 1 create call, got %d", len(fakeClient.createCalls))
	}
	if fakeClient.createCalls[0].org != "acme" {
		t.Errorf("expected org acme, got %s", fakeClient.createCalls[0].org)
	}
	if fakeClient.createCalls[0].opts.Name != "new-project" {
		t.Errorf("expected name new-project, got %s", fakeClient.createCalls[0].opts.Name)
	}
}

// TestProjectsCreate_FailsWhenNoOrg tests that create fails when no org available.
func TestProjectsCreate_FailsWhenNoOrg(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "") // empty default_org
	out := &bytes.Buffer{}

	cmd := &ProjectsCreateCmd{
		Name:          "new-project",
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
	if !strings.Contains(err.Error(), "organization is required") {
		t.Errorf("expected 'organization is required' error, got: %v", err)
	}
	// Verify it's NOT a RuntimeError - should be exit code 1 (usage error per PRD)
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		t.Errorf("expected plain error (exit code 1), got RuntimeError (exit code 2)")
	}
}

// TestProjectsUpdate_JSON tests updating a project.
func TestProjectsUpdate_JSON(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		project: &tfe.Project{
			ID:          "prj-123",
			Name:        "updated-project",
			Description: "Updated description",
		},
	}

	cmd := &ProjectsUpdateCmd{
		ID:            "prj-123",
		Name:          "updated-project",
		Description:   "Updated description",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(fakeClient.updateCalls))
	}
	if fakeClient.updateCalls[0].id != "prj-123" {
		t.Errorf("expected id prj-123, got %s", fakeClient.updateCalls[0].id)
	}
	if *fakeClient.updateCalls[0].opts.Name != "updated-project" {
		t.Errorf("expected name updated-project, got %s", *fakeClient.updateCalls[0].opts.Name)
	}
}

// TestProjectsDelete_PromptsWithoutForce tests that delete prompts without --force.
func TestProjectsDelete_PromptsWithoutForce(t *testing.T) {
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

// TestProjectsDelete_WithForce tests that delete bypasses prompt with --force.
func TestProjectsDelete_WithForce(t *testing.T) {
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
		prompter: &failingPrompter{}, // Would fail if called
	}

	cli := &CLI{Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "prj-123" {
		t.Errorf("expected delete call for prj-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestProjectsDelete_JSON tests that delete returns proper JSON on success.
func TestProjectsDelete_JSON(t *testing.T) {
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

// TestProjectsList_APIError tests that API errors are surfaced.
func TestProjectsList_APIError(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		listErr: errors.New("unauthorized"),
	}

	cmd := &ProjectsListCmd{
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
	if !strings.Contains(err.Error(), "failed to list projects") {
		t.Errorf("expected error message about list failure, got: %v", err)
	}
}

// TestProjectsGet_NotFound tests 404 error handling.
func TestProjectsGet_NotFound(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		readErr: tfe.ErrResourceNotFound,
	}

	cmd := &ProjectsGetCmd{
		ID:            "nonexistent",
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
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("expected Not Found error, got: %v", err)
	}
}

// TestProjectsList_FailsWhenSettingsMissing tests error when settings don't exist.
func TestProjectsList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir() // Empty dir, no settings
	out := &bytes.Buffer{}

	cmd := &ProjectsListCmd{
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      out,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tfccli init") {
		t.Errorf("expected error suggesting tfccli init, got: %v", err)
	}
}

// TestProjectsDelete_ConfirmYes tests that delete proceeds when user confirms.
func TestProjectsDelete_ConfirmYes(t *testing.T) {
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
		prompter: &acceptingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "prj-123" {
		t.Errorf("expected delete call for prj-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestProjectsUpdate_FailsWhenNoChanges tests that update requires at least one field.
func TestProjectsUpdate_FailsWhenNoChanges(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{}

	cmd := &ProjectsUpdateCmd{
		ID:            "prj-123",
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
	if !strings.Contains(err.Error(), "at least one of --name or --description is required") {
		t.Errorf("expected validation error, got: %v", err)
	}
	// Verify no API calls were made
	if len(fakeClient.updateCalls) != 0 {
		t.Errorf("expected no update calls, got: %v", fakeClient.updateCalls)
	}
}

// TestProjectsGet_Table tests that get outputs project details in table format.
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

// TestProjectsUpdate_Table tests that update outputs a success message in table mode.
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

	outStr := out.String()
	if !strings.Contains(outStr, "updated-project") || !strings.Contains(outStr, "updated") {
		t.Errorf("expected success message with project name, got: %s", outStr)
	}
}

// TestProjectsDelete_Table tests that delete outputs a success message in table mode.
func TestProjectsDelete_Table(t *testing.T) {
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
		prompter: &failingPrompter{},
	}

	cli := &CLI{OutputFormat: "table", Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "prj-123") || !strings.Contains(outStr, "deleted") {
		t.Errorf("expected success message with project ID, got: %s", outStr)
	}
}

// TestProjectsCreate_Table tests that create outputs a success message in table mode.
func TestProjectsCreate_Table(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		project: &tfe.Project{
			ID:   "prj-new",
			Name: "new-project",
		},
	}

	cmd := &ProjectsCreateCmd{
		Name:          "new-project",
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

	if !strings.Contains(out.String(), "new-project") || !strings.Contains(out.String(), "created") {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

// TestProjectsCreate_APIError tests that API errors during create are surfaced.
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

// TestProjectsUpdate_APIError tests that API errors during update are surfaced.
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

// TestProjectsDelete_APIError tests that API errors during delete are surfaced.
func TestProjectsDelete_APIError(t *testing.T) {
	tmpDir, resolver := setupProjectsTestSettings(t, "acme")
	out := &bytes.Buffer{}

	fakeClient := &fakeProjectsClient{
		deleteErr: errors.New("cannot delete project with workspaces"),
	}

	cmd := &ProjectsDeleteCmd{
		ID:            "prj-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (projectsClient, error) {
			return fakeClient, nil
		},
		prompter: &failingPrompter{},
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

// TestProjectsDelete_PromptError tests that prompter errors are surfaced.
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
