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

// fakeVariablesClient implements variablesClient for testing.
type fakeVariablesClient struct {
	variables  []*tfe.Variable
	variable   *tfe.Variable
	listErr    error
	readErr    error
	createErr  error
	updateErr  error
	deleteErr  error
	deleted    bool
	lastCreate tfe.VariableCreateOptions
	lastUpdate tfe.VariableUpdateOptions
}

func (c *fakeVariablesClient) List(_ context.Context, _ string, _ *tfe.VariableListOptions) ([]*tfe.Variable, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.variables, nil
}

func (c *fakeVariablesClient) Read(_ context.Context, _, _ string) (*tfe.Variable, error) {
	if c.readErr != nil {
		return nil, c.readErr
	}
	return c.variable, nil
}

func (c *fakeVariablesClient) Create(_ context.Context, _ string, opts tfe.VariableCreateOptions) (*tfe.Variable, error) {
	c.lastCreate = opts
	if c.createErr != nil {
		return nil, c.createErr
	}
	return c.variable, nil
}

func (c *fakeVariablesClient) Update(_ context.Context, _, _ string, opts tfe.VariableUpdateOptions) (*tfe.Variable, error) {
	c.lastUpdate = opts
	if c.updateErr != nil {
		return nil, c.updateErr
	}
	return c.variable, nil
}

func (c *fakeVariablesClient) Delete(_ context.Context, _, _ string) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	c.deleted = true
	return nil
}

// varsTestEnv implements auth.EnvGetter for testing.
type varsTestEnv struct {
	vars map[string]string
}

func (e *varsTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// varsTestFS implements auth.FSReader for testing.
type varsTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *varsTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *varsTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// varsAcceptingPrompter always returns true for confirms.
type varsAcceptingPrompter struct{}

func (p *varsAcceptingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *varsAcceptingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return true, nil
}

func (p *varsAcceptingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// varsRejectingPrompter always returns false for confirms.
type varsRejectingPrompter struct{}

func (p *varsRejectingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *varsRejectingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, nil
}

func (p *varsRejectingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// setupVariablesTestSettings creates test settings with token and returns the temp directory and token resolver.
func setupVariablesTestSettings(t *testing.T) (string, *auth.TokenResolver) {
	t.Helper()
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

	// Create fake env with token
	fakeEnv := &varsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFS := &varsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}
	return tmpDir, resolver
}

func TestWorkspaceVariablesList_JSON(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variables: []*tfe.Variable{
			{ID: "var-1", Key: "FOO", Category: tfe.CategoryEnv, Sensitive: false, HCL: false},
			{ID: "var-2", Key: "BAR", Category: tfe.CategoryTerraform, Sensitive: true, HCL: true},
		},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesListCmd{
		WorkspaceID:   "ws-123",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("expected data array in output")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 variables, got %d", len(data))
	}
}

func TestWorkspaceVariablesList_Table(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variables: []*tfe.Variable{
			{ID: "var-1", Key: "FOO", Category: tfe.CategoryEnv, Sensitive: false, HCL: false},
		},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "KEY") {
		t.Error("expected table headers in output")
	}
	if !strings.Contains(out, "var-1") || !strings.Contains(out, "FOO") {
		t.Error("expected variable data in output")
	}
}

func TestWorkspaceVariablesList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	// No settings file created

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesListCmd{
		WorkspaceID: "ws-123",
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
			return &fakeVariablesClient{}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings missing")
	}
	if !strings.Contains(err.Error(), "tfc init") {
		t.Errorf("expected error to suggest 'tfc init', got: %v", err)
	}
}

func TestWorkspaceVariablesCreate_JSON(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-new", Key: "NEW_VAR", Category: tfe.CategoryEnv},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesCreateCmd{
		WorkspaceID:   "ws-123",
		Key:           "NEW_VAR",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object in output")
	}
	if data["id"] != "var-new" {
		t.Errorf("expected id 'var-new', got %v", data["id"])
	}

	// Verify create options
	if *fakeClient.lastCreate.Key != "NEW_VAR" {
		t.Errorf("expected key NEW_VAR, got %v", *fakeClient.lastCreate.Key)
	}
	if *fakeClient.lastCreate.Value != "some-value" {
		t.Errorf("expected value 'some-value', got %v", *fakeClient.lastCreate.Value)
	}
}

func TestWorkspaceVariablesCreate_WithSensitiveAndHCL(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-new", Key: "SECRET", Category: tfe.CategoryEnv, Sensitive: true, HCL: true},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesCreateCmd{
		WorkspaceID:   "ws-123",
		Key:           "SECRET",
		Value:         "secret-value",
		Category:      "env",
		Sensitive:     true,
		HCL:           true,
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify create options include sensitive and HCL
	if *fakeClient.lastCreate.Sensitive != true {
		t.Error("expected sensitive=true")
	}
	if *fakeClient.lastCreate.HCL != true {
		t.Error("expected hcl=true")
	}
}

func TestWorkspaceVariablesCreate_Table(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-new", Key: "NEW_VAR", Category: tfe.CategoryEnv},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesCreateCmd{
		WorkspaceID:   "ws-123",
		Key:           "NEW_VAR",
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

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "created") {
		t.Error("expected success message")
	}
	if !strings.Contains(out, "var-new") {
		t.Error("expected variable ID in output")
	}
}

func TestWorkspaceVariablesUpdate_JSON(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-1", Key: "UPDATED_KEY", Category: tfe.CategoryEnv},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesUpdateCmd{
		VariableID:    "var-1",
		WorkspaceID:   "ws-123",
		Key:           "UPDATED_KEY",
		Value:         "new-value",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object in output")
	}
	if data["key"] != "UPDATED_KEY" {
		t.Errorf("expected key 'UPDATED_KEY', got %v", data["key"])
	}

	// Verify update options
	if *fakeClient.lastUpdate.Key != "UPDATED_KEY" {
		t.Errorf("expected key UPDATED_KEY in update, got %v", *fakeClient.lastUpdate.Key)
	}
}

func TestWorkspaceVariablesDelete_PromptsWithoutForce(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{}

	var buf bytes.Buffer
	prompter := &varsRejectingPrompter{} // User answers "no"
	cmd := &WorkspaceVariablesDeleteCmd{
		VariableID:    "var-1",
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
			return fakeClient, nil
		},
		prompter: prompter,
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.deleted {
		t.Error("expected delete to NOT be called when user answers no")
	}

	out := buf.String()
	if !strings.Contains(out, "Aborting") {
		t.Error("expected abort message")
	}
}

func TestWorkspaceVariablesDelete_WithForce(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{}

	var buf bytes.Buffer
	forceTrue := true
	cmd := &WorkspaceVariablesDeleteCmd{
		VariableID:    "var-1",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.deleted {
		t.Error("expected delete to be called with --force")
	}
}

func TestWorkspaceVariablesDelete_JSON(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{}

	var buf bytes.Buffer
	forceTrue := true
	cmd := &WorkspaceVariablesDeleteCmd{
		VariableID:    "var-1",
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

	cli := &CLI{OutputFormat: "json", Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected meta object in output")
	}
	if status, ok := meta["status"].(float64); !ok || int(status) != 204 {
		t.Errorf("expected status 204, got %v", meta["status"])
	}
}

func TestWorkspaceVariablesDelete_ConfirmYes(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{}

	var buf bytes.Buffer
	prompter := &varsAcceptingPrompter{} // User answers "yes"
	cmd := &WorkspaceVariablesDeleteCmd{
		VariableID:    "var-1",
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (variablesClient, error) {
			return fakeClient, nil
		},
		prompter: prompter,
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeClient.deleted {
		t.Error("expected delete to be called when user confirms")
	}
}

func TestWorkspaceVariablesList_APIError(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		listErr: errors.New("workspace not found"),
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesListCmd{
		WorkspaceID:   "ws-invalid",
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
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "failed to list variables") {
		t.Errorf("expected 'failed to list variables' in error, got: %v", err)
	}
}

func TestWorkspaceVariablesCreate_TerraformCategory(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-tf", Key: "TF_VAR", Category: tfe.CategoryTerraform},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesCreateCmd{
		WorkspaceID:   "ws-123",
		Key:           "TF_VAR",
		Value:         "terraform-value",
		Category:      "terraform",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify category is terraform
	if *fakeClient.lastCreate.Category != tfe.CategoryTerraform {
		t.Errorf("expected category terraform, got %v", *fakeClient.lastCreate.Category)
	}
}

func TestWorkspaceVariablesUpdate_PartialUpdate(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{
		variable: &tfe.Variable{ID: "var-1", Key: "EXISTING", Value: "new-val", Category: tfe.CategoryEnv},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesUpdateCmd{
		VariableID:    "var-1",
		WorkspaceID:   "ws-123",
		Value:         "new-val", // Only updating value
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only value was set in update options
	if fakeClient.lastUpdate.Key != nil {
		t.Error("expected key to not be set in partial update")
	}
	if *fakeClient.lastUpdate.Value != "new-val" {
		t.Errorf("expected value 'new-val', got %v", *fakeClient.lastUpdate.Value)
	}
}

// TestWorkspaceVariablesUpdate_FailsWhenNoFields tests that update fails when no fields provided.
func TestWorkspaceVariablesUpdate_FailsWhenNoFields(t *testing.T) {
	tmpDir, resolver := setupVariablesTestSettings(t)

	fakeClient := &fakeVariablesClient{}

	var buf bytes.Buffer
	cmd := &WorkspaceVariablesUpdateCmd{
		VariableID:  "var-123",
		WorkspaceID: "ws-123",
		// No update fields provided (Key, Value, Description, Sensitive, HCL all empty/nil)
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
		t.Fatal("expected error when no fields provided, got nil")
	}

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	// Verify error message
	if !strings.Contains(err.Error(), "at least one of") {
		t.Errorf("expected 'at least one of' error message, got: %v", err)
	}
}

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

	// Verify it's a RuntimeError for exit code 2
	var runtimeErr internalcmd.RuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected RuntimeError, got %T", err)
	}

	if !strings.Contains(err.Error(), "failed to prompt for confirmation") {
		t.Errorf("expected prompt error, got: %v", err)
	}
}
