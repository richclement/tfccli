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
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeWorkspaceResourcesClient implements workspaceResourcesClient for testing.
type fakeWorkspaceResourcesClient struct {
	resources []*tfe.WorkspaceResource
	listErr   error
}

func (c *fakeWorkspaceResourcesClient) List(_ context.Context, _ string, _ *tfe.WorkspaceResourceListOptions) ([]*tfe.WorkspaceResource, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.resources, nil
}

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

// setupWorkspaceResourcesTestSettings creates test settings with token and returns the temp directory and token resolver.
func setupWorkspaceResourcesTestSettings(t *testing.T) (string, *auth.TokenResolver) {
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
	return tmpDir, resolver
}

func TestWorkspaceResourcesList_JSON(t *testing.T) {
	tmpDir, resolver := setupWorkspaceResourcesTestSettings(t)

	fakeClient := &fakeWorkspaceResourcesClient{
		resources: []*tfe.WorkspaceResource{
			{ID: "res-1", Address: "aws_instance.web", Name: "web", ProviderType: "aws_instance", Provider: "aws"},
			{ID: "res-2", Address: "aws_s3_bucket.data", Name: "data", ProviderType: "aws_s3_bucket", Provider: "aws"},
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

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Verify top-level "data" field exists (Gherkin: Output JSON is raw JSON:API)
	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("expected data array in output")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 resources, got %d", len(data))
	}

	// Verify first resource fields
	first := data[0].(map[string]any)
	if first["id"] != "res-1" {
		t.Errorf("expected id 'res-1', got %v", first["id"])
	}
	if first["address"] != "aws_instance.web" {
		t.Errorf("expected address 'aws_instance.web', got %v", first["address"])
	}
}

func TestWorkspaceResourcesList_Table(t *testing.T) {
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
	// Verify table headers (RESOURCE-TYPE and PROVIDER columns)
	if !strings.Contains(out, "ID") || !strings.Contains(out, "RESOURCE-TYPE") || !strings.Contains(out, "NAME") || !strings.Contains(out, "PROVIDER") {
		t.Errorf("expected table headers in output, got: %s", out)
	}
	// Verify resource data
	if !strings.Contains(out, "res-1") || !strings.Contains(out, "web") {
		t.Errorf("expected resource data in output, got: %s", out)
	}
}

func TestWorkspaceResourcesList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	// No settings file created

	var buf bytes.Buffer
	cmd := &WorkspaceResourcesListCmd{
		WorkspaceID: "ws-123",
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
			return &fakeWorkspaceResourcesClient{}, nil
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

func TestWorkspaceResourcesList_APIError(t *testing.T) {
	tmpDir, resolver := setupWorkspaceResourcesTestSettings(t)

	fakeClient := &fakeWorkspaceResourcesClient{
		listErr: errors.New("workspace not found"),
	}

	var buf bytes.Buffer
	cmd := &WorkspaceResourcesListCmd{
		WorkspaceID:   "ws-invalid",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "failed to list workspace resources") {
		t.Errorf("expected 'failed to list workspace resources' in error, got: %v", err)
	}
}

func TestWorkspaceResourcesList_EmptyResources(t *testing.T) {
	tmpDir, resolver := setupWorkspaceResourcesTestSettings(t)

	fakeClient := &fakeWorkspaceResourcesClient{
		resources: []*tfe.WorkspaceResource{}, // Empty list
	}

	var buf bytes.Buffer
	cmd := &WorkspaceResourcesListCmd{
		WorkspaceID:   "ws-empty",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(_ tfcapi.ClientConfig) (workspaceResourcesClient, error) {
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
	if len(data) != 0 {
		t.Errorf("expected 0 resources, got %d", len(data))
	}
}

func TestWorkspaceResourcesList_ContextOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings with multiple contexts
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "app.terraform.io",
				LogLevel: "info",
			},
			"other": {
				Address:  "tfe.example.com",
				LogLevel: "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save test settings: %v", err)
	}

	// Create fake env with token for both hosts
	fakeEnv := &wsrTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "default-token",
			"TF_TOKEN_tfe_example_com":  "other-token",
		},
	}
	fakeFS := &wsrTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	fakeClient := &fakeWorkspaceResourcesClient{
		resources: []*tfe.WorkspaceResource{},
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

	// Use --context flag to select "other" context
	cli := &CLI{OutputFormat: "json", Context: "other"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Command should succeed with the other context
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if _, ok := result["data"]; !ok {
		t.Fatal("expected data in output")
	}
}

func TestWorkspaceResourcesList_AddressOverride(t *testing.T) {
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

	// Create fake env with token for custom address
	fakeEnv := &wsrTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_tfe_io": "custom-token",
		},
	}
	fakeFS := &wsrTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var capturedAddress string
	fakeClient := &fakeWorkspaceResourcesClient{
		resources: []*tfe.WorkspaceResource{},
	}

	var buf bytes.Buffer
	cmd := &WorkspaceResourcesListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &buf,
		clientFactory: func(cfg tfcapi.ClientConfig) (workspaceResourcesClient, error) {
			capturedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	// Use --address flag to override
	cli := &CLI{OutputFormat: "json", Address: "custom.tfe.io"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAddress != "custom.tfe.io" {
		t.Errorf("expected address 'custom.tfe.io', got %q", capturedAddress)
	}
}

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
