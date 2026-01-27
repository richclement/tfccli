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

// fakeOrgsClient is a test double for orgsClient.
type fakeOrgsClient struct {
	orgs      []*tfe.Organization
	org       *tfe.Organization
	listErr   error
	readErr   error
	createErr error
	updateErr error
	deleteErr error

	// Track calls for assertions
	listCalls   int
	readCalls   []string
	createCalls []tfe.OrganizationCreateOptions
	updateCalls []struct {
		name string
		opts tfe.OrganizationUpdateOptions
	}
	deleteCalls []string
}

func (f *fakeOrgsClient) List(_ context.Context, _ *tfe.OrganizationListOptions) ([]*tfe.Organization, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.orgs, nil
}

func (f *fakeOrgsClient) Read(_ context.Context, name string) (*tfe.Organization, error) {
	f.readCalls = append(f.readCalls, name)
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.org, nil
}

func (f *fakeOrgsClient) Create(_ context.Context, opts tfe.OrganizationCreateOptions) (*tfe.Organization, error) {
	f.createCalls = append(f.createCalls, opts)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.org, nil
}

func (f *fakeOrgsClient) Update(_ context.Context, name string, opts tfe.OrganizationUpdateOptions) (*tfe.Organization, error) {
	f.updateCalls = append(f.updateCalls, struct {
		name string
		opts tfe.OrganizationUpdateOptions
	}{name, opts})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.org, nil
}

func (f *fakeOrgsClient) Delete(_ context.Context, name string) error {
	f.deleteCalls = append(f.deleteCalls, name)
	return f.deleteErr
}

// orgsTestEnv implements auth.EnvGetter for testing.
type orgsTestEnv struct {
	vars map[string]string
}

func (e *orgsTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// orgsTestFS implements auth.FSReader for testing.
type orgsTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *orgsTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *orgsTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// setupOrgsTestSettings creates test settings and returns the temp directory and token resolver.
func setupOrgsTestSettings(t *testing.T) (string, *auth.TokenResolver) {
	t.Helper()
	tmpDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    "app.terraform.io",
				DefaultOrg: "test-org",
				LogLevel:   "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save test settings: %v", err)
	}

	// Create fake env with token
	fakeEnvMap := &orgsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFSMap := &orgsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap}
	return tmpDir, resolver
}

// TestOrganizationsList_JSON tests that list returns organizations as JSON.
func TestOrganizationsList_JSON(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{
			{Name: "org-1", Email: "admin1@example.com", ExternalID: "ext-1"},
			{Name: "org-2", Email: "admin2@example.com", ExternalID: "ext-2"},
		},
	}

	cmd := &OrganizationsListCmd{
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeClient.listCalls != 1 {
		t.Errorf("expected 1 list call, got %d", fakeClient.listCalls)
	}

	// Parse JSON output
	var result struct {
		Data []*tfe.Organization `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 organizations, got %d", len(result.Data))
	}
}

// TestOrganizationsList_Table tests that list returns organizations as table.
func TestOrganizationsList_Table(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{
			{Name: "org-1", Email: "admin1@example.com", ExternalID: "ext-1"},
		},
	}

	cmd := &OrganizationsListCmd{
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
	if !strings.Contains(outStr, "NAME") || !strings.Contains(outStr, "EMAIL") {
		t.Errorf("expected table headers, got: %s", outStr)
	}
	if !strings.Contains(outStr, "org-1") {
		t.Errorf("expected organization name in output, got: %s", outStr)
	}
}

// TestOrganizationsList_EmptyTable tests that empty list shows a message in table mode.
func TestOrganizationsList_EmptyTable(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{}, // Empty list
	}

	cmd := &OrganizationsListCmd{
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
	if !strings.Contains(outStr, "No organizations found.") {
		t.Errorf("expected 'No organizations found.' message, got: %s", outStr)
	}
}

// TestOrganizationsList_EmptyJSON tests that empty list returns empty JSON array.
func TestOrganizationsList_EmptyJSON(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{}, // Empty list
	}

	cmd := &OrganizationsListCmd{
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse JSON output - should have empty data array
	var result struct {
		Data []*tfe.Organization `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if len(result.Data) != 0 {
		t.Errorf("expected 0 organizations, got %d", len(result.Data))
	}
}

// TestOrganizationsGet_JSON tests getting an organization by name.
func TestOrganizationsGet_JSON(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		org: &tfe.Organization{
			Name:      "org-123",
			Email:     "admin@example.com",
			CreatedAt: time.Now(),
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.readCalls) != 1 || fakeClient.readCalls[0] != "org-123" {
		t.Errorf("expected read call for org-123, got: %v", fakeClient.readCalls)
	}

	// Parse JSON output
	var result struct {
		Data *tfe.Organization `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if result.Data.Name != "org-123" {
		t.Errorf("expected org-123, got %s", result.Data.Name)
	}
}

// TestOrganizationsGet_Table tests getting an organization with table output.
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

	if len(fakeClient.readCalls) != 1 || fakeClient.readCalls[0] != "org-123" {
		t.Errorf("expected read call for org-123, got: %v", fakeClient.readCalls)
	}

	outStr := out.String()
	// Verify table headers
	if !strings.Contains(outStr, "FIELD") || !strings.Contains(outStr, "VALUE") {
		t.Errorf("expected table headers FIELD and VALUE, got: %s", outStr)
	}
	// Verify field values
	if !strings.Contains(outStr, "org-123") {
		t.Errorf("expected org name in output, got: %s", outStr)
	}
	if !strings.Contains(outStr, "admin@example.com") {
		t.Errorf("expected email in output, got: %s", outStr)
	}
	if !strings.Contains(outStr, "ext-123") {
		t.Errorf("expected external ID in output, got: %s", outStr)
	}
	// CreatedAt should be formatted
	if !strings.Contains(outStr, "2024") {
		t.Errorf("expected created at year in output, got: %s", outStr)
	}
}

// TestOrganizationsDelete_PromptsWithoutForce tests that delete prompts without --force.
func TestOrganizationsDelete_PromptsWithoutForce(t *testing.T) {
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

// TestOrganizationsDelete_WithForce tests that delete bypasses prompt with --force.
func TestOrganizationsDelete_WithForce(t *testing.T) {
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
		prompter: &failingPrompter{}, // Would fail if called
	}

	cli := &CLI{Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "org-123" {
		t.Errorf("expected delete call for org-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestOrganizationsDelete_JSON tests that delete returns proper JSON on success.
func TestOrganizationsDelete_JSON(t *testing.T) {
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

// TestOrganizationsCreate_JSON tests creating an organization.
func TestOrganizationsCreate_JSON(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		org: &tfe.Organization{
			Name:  "new-org",
			Email: "admin@example.com",
		},
	}

	cmd := &OrganizationsCreateCmd{
		Name:          "new-org",
		Email:         "admin@example.com",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.createCalls) != 1 {
		t.Errorf("expected 1 create call, got %d", len(fakeClient.createCalls))
	}
	if *fakeClient.createCalls[0].Name != "new-org" {
		t.Errorf("expected name new-org, got %s", *fakeClient.createCalls[0].Name)
	}
	if *fakeClient.createCalls[0].Email != "admin@example.com" {
		t.Errorf("expected email admin@example.com, got %s", *fakeClient.createCalls[0].Email)
	}
}

// TestOrganizationsUpdate_JSON tests updating an organization.
func TestOrganizationsUpdate_JSON(t *testing.T) {
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fakeClient.updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(fakeClient.updateCalls))
	}
	if fakeClient.updateCalls[0].name != "org-123" {
		t.Errorf("expected name org-123, got %s", fakeClient.updateCalls[0].name)
	}
	if *fakeClient.updateCalls[0].opts.Email != "newemail@example.com" {
		t.Errorf("expected email newemail@example.com, got %s", *fakeClient.updateCalls[0].opts.Email)
	}
}

// TestOrganizationsUpdate_NoFieldsProvided tests that update fails when no fields are provided.
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

	// Should return an error when no fields provided
	if err == nil {
		t.Fatal("expected error when no fields provided, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("expected 'nothing to update' error, got: %v", err)
	}

	// Should NOT have called the API
	if len(fakeClient.updateCalls) != 0 {
		t.Errorf("expected no update calls when no fields provided, got: %v", fakeClient.updateCalls)
	}
}

// TestOrganizationsUpdate_Table tests that update outputs a success message in table mode.
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

	outStr := out.String()
	if !strings.Contains(outStr, "org-123") || !strings.Contains(outStr, "updated") {
		t.Errorf("expected success message with org name and 'updated', got: %s", outStr)
	}
}

// TestOrganizationsList_APIError tests that API errors are surfaced.
func TestOrganizationsList_APIError(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		listErr: errors.New("unauthorized"),
	}

	cmd := &OrganizationsListCmd{
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
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to list organizations") {
		t.Errorf("expected error message about list failure, got: %v", err)
	}
}

// TestOrganizationsList_FailsWhenSettingsMissing tests error when settings don't exist.
func TestOrganizationsList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir() // Empty dir, no settings
	out := &bytes.Buffer{}

	cmd := &OrganizationsListCmd{
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

// TestOrganizationsGet_NotFound tests 404 error handling.
func TestOrganizationsGet_NotFound(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		readErr: tfe.ErrResourceNotFound,
	}

	cmd := &OrganizationsGetCmd{
		Name:          "nonexistent",
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
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("expected Not Found error, got: %v", err)
	}
}

// TestOrganizationsDelete_ConfirmYes tests that delete proceeds when user confirms.
func TestOrganizationsDelete_ConfirmYes(t *testing.T) {
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
		prompter: &acceptingPrompter{},
	}

	cli := &CLI{Force: false}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "org-123" {
		t.Errorf("expected delete call for org-123, got: %v", fakeClient.deleteCalls)
	}
}

// TestOrganizations_ContextOverride tests that --context flag works.
func TestOrganizations_ContextOverride(t *testing.T) {
	tmpDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "app.terraform.io",
				LogLevel: "info",
			},
			"prod": {
				Address:  "tfe.example.com",
				LogLevel: "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save test settings: %v", err)
	}

	// Token for both addresses
	fakeEnvMap := &orgsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
			"TF_TOKEN_tfe_example_com":  "prod-token",
		},
	}
	fakeFSMap := &orgsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap}

	out := &bytes.Buffer{}
	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{},
	}

	var capturedAddress string
	cmd := &OrganizationsListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(cfg tfcapi.ClientConfig) (orgsClient, error) {
			capturedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Context: "prod", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAddress != "tfe.example.com" {
		t.Errorf("expected address tfe.example.com, got %s", capturedAddress)
	}
}

// TestOrganizations_AddressOverride tests that --address flag works.
func TestOrganizations_AddressOverride(t *testing.T) {
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

	// Token for both addresses
	fakeEnvMap := &orgsTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_tfe_io": "custom-token",
		},
	}
	fakeFSMap := &orgsTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap}

	out := &bytes.Buffer{}
	fakeClient := &fakeOrgsClient{
		orgs: []*tfe.Organization{},
	}

	var capturedAddress string
	cmd := &OrganizationsListCmd{
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(cfg tfcapi.ClientConfig) (orgsClient, error) {
			capturedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Address: "custom.tfe.io", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAddress != "custom.tfe.io" {
		t.Errorf("expected address custom.tfe.io, got %s", capturedAddress)
	}
}

// TestOrganizationsDelete_PrompterError tests that prompter errors are surfaced.
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

	cli := &CLI{Force: false} // Not forced, so prompter will be called
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to prompt") {
		t.Errorf("expected 'failed to prompt' error, got: %v", err)
	}

	// Should NOT have called delete since prompter failed
	if len(fakeClient.deleteCalls) != 0 {
		t.Errorf("expected no delete calls when prompter fails, got: %v", fakeClient.deleteCalls)
	}
}

// TestOrganizationsList_ClientFactoryError tests that client factory errors are surfaced.
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
		t.Errorf("expected 'failed to create client' error, got: %v", err)
	}
}

// TestOrganizationsGet_ClientFactoryError tests that client factory errors are surfaced.
func TestOrganizationsGet_ClientFactoryError(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	cmd := &OrganizationsGetCmd{
		Name:          "org-123",
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
		t.Errorf("expected 'failed to create client' error, got: %v", err)
	}
}

// TestOrganizationsCreate_ClientFactoryError tests that client factory errors are surfaced.
func TestOrganizationsCreate_ClientFactoryError(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	cmd := &OrganizationsCreateCmd{
		Name:          "new-org",
		Email:         "admin@example.com",
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
		t.Errorf("expected 'failed to create client' error, got: %v", err)
	}
}

// TestOrganizationsUpdate_ClientFactoryError tests that client factory errors are surfaced.
func TestOrganizationsUpdate_ClientFactoryError(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	cmd := &OrganizationsUpdateCmd{
		Name:          "org-123",
		Email:         "newemail@example.com",
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
		t.Errorf("expected 'failed to create client' error, got: %v", err)
	}
}

// TestOrganizationsDelete_ClientFactoryError tests that client factory errors are surfaced.
func TestOrganizationsDelete_ClientFactoryError(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	cmd := &OrganizationsDeleteCmd{
		Name:          "org-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        out,
		clientFactory: func(_ tfcapi.ClientConfig) (orgsClient, error) {
			return nil, errors.New("connection refused")
		},
		prompter: &failingPrompter{}, // Would fail if called
	}

	cli := &CLI{Force: true}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create client") {
		t.Errorf("expected 'failed to create client' error, got: %v", err)
	}
}

// TestOrganizationsCreate_Table tests that create outputs a success message in table mode.
func TestOrganizationsCreate_Table(t *testing.T) {
	tmpDir, resolver := setupOrgsTestSettings(t)
	out := &bytes.Buffer{}

	fakeClient := &fakeOrgsClient{
		org: &tfe.Organization{
			Name:  "new-org",
			Email: "admin@example.com",
		},
	}

	cmd := &OrganizationsCreateCmd{
		Name:          "new-org",
		Email:         "admin@example.com",
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

	if !strings.Contains(out.String(), "new-org") || !strings.Contains(out.String(), "created") {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

// TestOrganizationsDelete_Table tests that delete outputs a success message in table mode.
func TestOrganizationsDelete_Table(t *testing.T) {
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
		prompter: &failingPrompter{}, // Would fail if called
	}

	cli := &CLI{OutputFormat: "table", Force: true}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called delete
	if len(fakeClient.deleteCalls) != 1 || fakeClient.deleteCalls[0] != "org-123" {
		t.Errorf("expected delete call for org-123, got: %v", fakeClient.deleteCalls)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "org-123") || !strings.Contains(outStr, "deleted") {
		t.Errorf("expected success message with org name and 'deleted', got: %s", outStr)
	}
}
