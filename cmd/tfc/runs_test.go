package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeRunsClient implements runsClient for testing.
type fakeRunsClient struct {
	runs              []*tfe.Run
	run               *tfe.Run
	createdRun        *tfe.Run
	listErr           error
	readErr           error
	createErr         error
	applyErr          error
	discardErr        error
	cancelErr         error
	forceCancelErr    error
	applyCalled       bool
	discardCalled     bool
	cancelCalled      bool
	forceCancelCalled bool
	createOpts        tfe.RunCreateOptions

	// Captured parameters for verification
	listWorkspaceID  string
	listOpts         *tfe.RunListOptions
	listLimit        int
	readRunID        string
	applyRunID       string
	applyOpts        tfe.RunApplyOptions
	discardRunID     string
	discardOpts      tfe.RunDiscardOptions
	cancelRunID      string
	cancelOpts       tfe.RunCancelOptions
	forceCancelRunID string
	forceCancelOpts  tfe.RunForceCancelOptions
}

func (c *fakeRunsClient) List(_ context.Context, workspaceID string, opts *tfe.RunListOptions, limit int) ([]*tfe.Run, error) {
	c.listWorkspaceID = workspaceID
	c.listOpts = opts
	c.listLimit = limit
	if c.listErr != nil {
		return nil, c.listErr
	}
	// Respect limit if set
	if limit > 0 && len(c.runs) > limit {
		return c.runs[:limit], nil
	}
	return c.runs, nil
}

func (c *fakeRunsClient) Read(_ context.Context, runID string) (*tfe.Run, error) {
	c.readRunID = runID
	if c.readErr != nil {
		return nil, c.readErr
	}
	return c.run, nil
}

func (c *fakeRunsClient) Create(_ context.Context, opts tfe.RunCreateOptions) (*tfe.Run, error) {
	c.createOpts = opts
	if c.createErr != nil {
		return nil, c.createErr
	}
	return c.createdRun, nil
}

func (c *fakeRunsClient) Apply(_ context.Context, runID string, opts tfe.RunApplyOptions) error {
	c.applyRunID = runID
	c.applyOpts = opts
	c.applyCalled = true
	return c.applyErr
}

func (c *fakeRunsClient) Discard(_ context.Context, runID string, opts tfe.RunDiscardOptions) error {
	c.discardRunID = runID
	c.discardOpts = opts
	c.discardCalled = true
	return c.discardErr
}

func (c *fakeRunsClient) Cancel(_ context.Context, runID string, opts tfe.RunCancelOptions) error {
	c.cancelRunID = runID
	c.cancelOpts = opts
	c.cancelCalled = true
	return c.cancelErr
}

func (c *fakeRunsClient) ForceCancel(_ context.Context, runID string, opts tfe.RunForceCancelOptions) error {
	c.forceCancelRunID = runID
	c.forceCancelOpts = opts
	c.forceCancelCalled = true
	return c.forceCancelErr
}

// runsTestEnv implements auth.EnvGetter for testing.
type runsTestEnv struct {
	vars map[string]string
}

func (e *runsTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// runsTestFS implements auth.FSReader for testing.
type runsTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *runsTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *runsTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// runsAcceptingPrompter always returns true for confirms.
type runsAcceptingPrompter struct{}

func (p *runsAcceptingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *runsAcceptingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return true, nil
}

func (p *runsAcceptingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// runsRejectingPrompter always returns false for confirms.
type runsRejectingPrompter struct{}

func (p *runsRejectingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *runsRejectingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, nil
}

func (p *runsRejectingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// runsFailingPrompter returns an error to verify prompts are bypassed with --force.
type runsFailingPrompter struct{}

func (p *runsFailingPrompter) PromptString(_, _ string) (string, error) {
	return "", errors.New("should not be called with --force")
}

func (p *runsFailingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, errors.New("should not be called with --force")
}

func (p *runsFailingPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
	return "", errors.New("should not be called with --force")
}

// runsErrorPrompter returns an error to verify error handling when prompter fails.
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

func setupRunsTest(t *testing.T) (string, *auth.TokenResolver) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create settings file
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

	// Create fake env with token
	fakeEnv := &runsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFS := &runsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}
	return tmpDir, resolver
}

func TestRunsList_JSON(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, Message: "Test run 1", CreatedAt: createdAt},
			{ID: "run-2", Status: tfe.RunApplied, Message: "Test run 2", CreatedAt: createdAt.Add(time.Hour)},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
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
	if len(data) != 2 {
		t.Errorf("expected 2 runs, got %d", len(data))
	}
}

func TestRunsList_Table(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, Message: "Test run 1", CreatedAt: createdAt},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
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
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") {
		t.Errorf("expected table headers, got: %s", out)
	}
	if !strings.Contains(out, "run-1") {
		t.Errorf("expected run ID in output, got: %s", out)
	}
}

func TestRunsList_EmptyList(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{}, // Empty list
	}

	t.Run("json output", func(t *testing.T) {
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
	})

	t.Run("table output", func(t *testing.T) {
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

		cli := &CLI{OutputFormat: "table"}
		err := cmd.Run(cli)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := stdout.String()
		// Table output should contain headers even with no data
		if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") {
			t.Errorf("expected table headers in empty output, got: %s", out)
		}
	})
}

func TestRunsList_WithLimit(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	// Create 5 runs for testing
	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, Message: "Run 1", CreatedAt: createdAt},
			{ID: "run-2", Status: tfe.RunApplied, Message: "Run 2", CreatedAt: createdAt},
			{ID: "run-3", Status: tfe.RunPlanning, Message: "Run 3", CreatedAt: createdAt},
			{ID: "run-4", Status: tfe.RunPending, Message: "Run 4", CreatedAt: createdAt},
			{ID: "run-5", Status: tfe.RunCanceled, Message: "Run 5", CreatedAt: createdAt},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
		Limit:         3,
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

	// Verify limit was passed to client
	if fakeClient.listLimit != 3 {
		t.Errorf("expected limit 3 to be passed to client, got %d", fakeClient.listLimit)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 3 {
		t.Errorf("expected 3 runs with limit, got %d", len(data))
	}
}

func TestRunsList_LimitZeroFetchesAll(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	// Create 5 runs for testing
	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, Message: "Run 1", CreatedAt: createdAt},
			{ID: "run-2", Status: tfe.RunApplied, Message: "Run 2", CreatedAt: createdAt},
			{ID: "run-3", Status: tfe.RunPlanning, Message: "Run 3", CreatedAt: createdAt},
			{ID: "run-4", Status: tfe.RunPending, Message: "Run 4", CreatedAt: createdAt},
			{ID: "run-5", Status: tfe.RunCanceled, Message: "Run 5", CreatedAt: createdAt},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
		Limit:         0, // 0 = all
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

	// Verify limit was passed as 0
	if fakeClient.listLimit != 0 {
		t.Errorf("expected limit 0 to be passed to client, got %d", fakeClient.listLimit)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 5 {
		t.Errorf("expected all 5 runs with limit=0, got %d", len(data))
	}
}

func TestRunsGet_JSON(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		run: &tfe.Run{
			ID:        "run-1",
			Status:    tfe.RunPlanned,
			Message:   "Test run",
			CreatedAt: createdAt,
			Source:    tfe.RunSourceAPI,
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", result["data"])
	}
	if data["id"] != "run-1" {
		t.Errorf("expected id run-1, got %v", data["id"])
	}
}

func TestRunsGet_JSON_WithWorkspace(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		run: &tfe.Run{
			ID:        "run-1",
			Status:    tfe.RunPlanned,
			Message:   "Test run",
			CreatedAt: createdAt,
			Source:    tfe.RunSourceAPI,
			Workspace: &tfe.Workspace{ID: "ws-test123"},
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", result["data"])
	}
	if data["workspace_id"] != "ws-test123" {
		t.Errorf("expected workspace_id ws-test123, got %v", data["workspace_id"])
	}
}

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
	if !strings.Contains(out, "planned") {
		t.Errorf("expected status 'planned' in output, got: %s", out)
	}
	if !strings.Contains(out, "Test run") {
		t.Errorf("expected message in output, got: %s", out)
	}
	if !strings.Contains(out, "tfe-api") {
		t.Errorf("expected source 'tfe-api' in output, got: %s", out)
	}
	if !strings.Contains(out, "2025-01-15") {
		t.Errorf("expected created_at date in output, got: %s", out)
	}
	if !strings.Contains(out, "Workspace ID") {
		t.Errorf("expected Workspace ID field in output, got: %s", out)
	}
	if !strings.Contains(out, "ws-test") {
		t.Errorf("expected workspace ID value in output, got: %s", out)
	}
}

func TestRunsCreate_JSON(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		createdRun: &tfe.Run{
			ID:        "run-new",
			Status:    tfe.RunPending,
			Message:   "Created via CLI",
			CreatedAt: createdAt,
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsCreateCmd{
		WorkspaceID:   "ws-test",
		Message:       "Created via CLI",
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

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", result["data"])
	}
	if data["id"] != "run-new" {
		t.Errorf("expected id run-new, got %v", data["id"])
	}

	// Verify create options
	if fakeClient.createOpts.Workspace == nil || fakeClient.createOpts.Workspace.ID != "ws-test" {
		t.Errorf("expected workspace ID ws-test in create options")
	}
	if fakeClient.createOpts.Message == nil || *fakeClient.createOpts.Message != "Created via CLI" {
		t.Errorf("expected message 'Created via CLI' in create options")
	}
}

func TestRunsCreate_Table(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		createdRun: &tfe.Run{
			ID:        "run-new",
			Status:    tfe.RunPending,
			CreatedAt: createdAt,
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsCreateCmd{
		WorkspaceID:   "ws-test",
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
	if !strings.Contains(out, "run-new") {
		t.Errorf("expected run ID in output, got: %s", out)
	}
}

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

func TestRunsApply_PromptsWithoutForce(t *testing.T) {
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
		prompter: &runsRejectingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.applyCalled {
		t.Error("expected apply NOT to be called when user declines")
	}
	if !strings.Contains(stdout.String(), "Aborting") {
		t.Errorf("expected abort message, got: %s", stdout.String())
	}
}

func TestRunsApply_WithForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsApplyCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
		prompter:  &runsFailingPrompter{}, // Should not be called
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.applyCalled {
		t.Error("expected apply to be called with --force")
	}

	// Verify JSON response
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
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
}

func TestRunsApply_ConfirmYes(t *testing.T) {
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
		prompter: &runsAcceptingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.applyCalled {
		t.Error("expected apply to be called when user confirms")
	}
}

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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.applyCalled {
		t.Error("expected apply to be called")
	}
	if fakeClient.applyOpts.Comment == nil || *fakeClient.applyOpts.Comment != "LGTM, applying" {
		t.Errorf("expected comment 'LGTM, applying' to be passed to API, got %v", fakeClient.applyOpts.Comment)
	}
}

func TestRunsDiscard_PromptsWithoutForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsDiscardCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		prompter: &runsRejectingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.discardCalled {
		t.Error("expected discard NOT to be called when user declines")
	}
}

func TestRunsDiscard_WithForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsDiscardCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.discardCalled {
		t.Error("expected discard to be called with --force")
	}
}

func TestRunsDiscard_WithComment(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsDiscardCmd{
		ID:            "run-1",
		Comment:       "Discarding due to failed review",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.discardCalled {
		t.Error("expected discard to be called")
	}
	if fakeClient.discardOpts.Comment == nil || *fakeClient.discardOpts.Comment != "Discarding due to failed review" {
		t.Errorf("expected comment to be passed to API, got %v", fakeClient.discardOpts.Comment)
	}
}

func TestRunsCancel_PromptsWithoutForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsCancelCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		prompter: &runsRejectingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.cancelCalled {
		t.Error("expected cancel NOT to be called when user declines")
	}
}

func TestRunsCancel_WithForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsCancelCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.cancelCalled {
		t.Error("expected cancel to be called with --force")
	}
}

func TestRunsCancel_WithComment(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsCancelCmd{
		ID:            "run-1",
		Comment:       "Cancelling to update configuration",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.cancelCalled {
		t.Error("expected cancel to be called")
	}
	if fakeClient.cancelOpts.Comment == nil || *fakeClient.cancelOpts.Comment != "Cancelling to update configuration" {
		t.Errorf("expected comment to be passed to API, got %v", fakeClient.cancelOpts.Comment)
	}
}

func TestRunsForceCancel_PromptsWithoutForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsForceCancelCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		prompter: &runsRejectingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.forceCancelCalled {
		t.Error("expected force-cancel NOT to be called when user declines")
	}
}

func TestRunsForceCancel_WithForce(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsForceCancelCmd{
		ID:            "run-1",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.forceCancelCalled {
		t.Error("expected force-cancel to be called with --force")
	}
}

func TestRunsForceCancel_WithComment(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	forceFlag := true
	cmd := &RunsForceCancelCmd{
		ID:            "run-1",
		Comment:       "Emergency force-cancel required",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
		forceFlag: &forceFlag,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.forceCancelCalled {
		t.Error("expected force-cancel to be called")
	}
	if fakeClient.forceCancelOpts.Comment == nil || *fakeClient.forceCancelOpts.Comment != "Emergency force-cancel required" {
		t.Errorf("expected comment to be passed to API, got %v", fakeClient.forceCancelOpts.Comment)
	}
}

func TestRunsList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir() // No settings file

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID: "ws-test",
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      &stdout,
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for missing settings")
	}
	if !strings.Contains(err.Error(), "tfc init") {
		t.Errorf("expected error to suggest tfc init, got: %v", err)
	}
}

func TestRunsList_APIError(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{
		listErr: errors.New("workspace not found"),
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
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
		t.Errorf("expected error to contain API error message, got: %v", err)
	}
}

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

func TestRunsGet_NotFound(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{
		readErr: tfe.ErrResourceNotFound,
	}

	var stdout bytes.Buffer
	cmd := &RunsGetCmd{
		ID:            "run-notfound",
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
		t.Fatal("expected error for not found")
	}
}

func TestRunsCreate_WithAutoApply(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		createdRun: &tfe.Run{
			ID:        "run-new",
			Status:    tfe.RunPending,
			CreatedAt: createdAt,
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsCreateCmd{
		WorkspaceID:   "ws-test",
		AutoApply:     true,
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

	// Verify auto-apply was set
	if fakeClient.createOpts.AutoApply == nil || !*fakeClient.createOpts.AutoApply {
		t.Error("expected auto-apply to be true in create options")
	}
}

func TestRuns_ContextOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings with multiple contexts
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "app.terraform.io",
				LogLevel: "info",
			},
			"staging": {
				Address:  "staging.terraform.io",
				LogLevel: "debug",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Token resolver that returns different tokens based on address
	fakeEnv := &runsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_staging_terraform_io": "staging-token",
		},
	}
	fakeFS := &runsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, CreatedAt: createdAt},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{Context: "staging", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRuns_AddressOverride(t *testing.T) {
	tmpDir, _ := setupRunsTest(t)

	// Token resolver that works with custom address
	fakeEnv := &runsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_terraform_io": "custom-token",
		},
	}
	fakeFS := &runsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	createdAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	fakeClient := &fakeRunsClient{
		runs: []*tfe.Run{
			{ID: "run-1", Status: tfe.RunPlanned, CreatedAt: createdAt},
		},
	}

	var stdout bytes.Buffer
	cmd := &RunsListCmd{
		WorkspaceID:   "ws-test",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (runsClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{Address: "custom.terraform.io", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestRunsDiscard_PrompterError(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsDiscardCmd{
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
	if fakeClient.discardCalled {
		t.Error("discard should not be called when prompt fails")
	}
}

func TestRunsCancel_PrompterError(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsCancelCmd{
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
	if fakeClient.cancelCalled {
		t.Error("cancel should not be called when prompt fails")
	}
}

func TestRunsForceCancel_PrompterError(t *testing.T) {
	tmpDir, resolver := setupRunsTest(t)

	fakeClient := &fakeRunsClient{}

	var stdout bytes.Buffer
	cmd := &RunsForceCancelCmd{
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
	if fakeClient.forceCancelCalled {
		t.Error("force-cancel should not be called when prompt fails")
	}
}
