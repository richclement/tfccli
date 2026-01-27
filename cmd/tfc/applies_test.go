package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeAppliesClient is a mock for testing.
type fakeAppliesClient struct {
	ReadFunc             func(ctx context.Context, applyID string) (*tfe.Apply, error)
	GetErroredStateURLFn func(ctx context.Context, applyID string) (string, error)
}

func (f *fakeAppliesClient) Read(ctx context.Context, applyID string) (*tfe.Apply, error) {
	if f.ReadFunc != nil {
		return f.ReadFunc(ctx, applyID)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeAppliesClient) GetErroredStateURL(ctx context.Context, applyID string) (string, error) {
	if f.GetErroredStateURLFn != nil {
		return f.GetErroredStateURLFn(ctx, applyID)
	}
	return "", errors.New("not implemented")
}

// appliesTestEnv implements auth.EnvGetter for testing.
type appliesTestEnv struct {
	vars map[string]string
}

func (e *appliesTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// appliesTestFS implements auth.FSReader for testing.
type appliesTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *appliesTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *appliesTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

func setupAppliesTest(t *testing.T) (string, *auth.TokenResolver) {
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
		t.Fatal(err)
	}

	fakeEnv := &appliesTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFS := &appliesTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}
	return tmpDir, resolver
}

func TestAppliesGet_JSON(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, applyID string) (*tfe.Apply, error) {
			if applyID != "apply-123" {
				return nil, errors.New("not found")
			}
			return &tfe.Apply{
				ID:                   "apply-123",
				Status:               tfe.ApplyFinished,
				ResourceAdditions:    5,
				ResourceChanges:      2,
				ResourceDestructions: 1,
				ResourceImports:      0,
				LogReadURL:           "https://example.com/logs",
			}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
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
		t.Fatal("expected 'data' field in JSON output")
	}
	if data["id"] != "apply-123" {
		t.Errorf("expected id=apply-123, got %v", data["id"])
	}
	if data["status"] != "finished" {
		t.Errorf("expected status=finished, got %v", data["status"])
	}
}

func TestAppliesGet_Table(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.Apply, error) {
			return &tfe.Apply{
				ID:                   "apply-456",
				Status:               tfe.ApplyRunning,
				ResourceAdditions:    3,
				ResourceChanges:      1,
				ResourceDestructions: 0,
				ResourceImports:      2,
			}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-456",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "FIELD") || !strings.Contains(out, "VALUE") {
		t.Error("expected table headers FIELD, VALUE")
	}
	if !strings.Contains(out, "apply-456") {
		t.Error("expected apply-456 in output")
	}
	if !strings.Contains(out, "running") {
		t.Error("expected status running in output")
	}
}

func TestAppliesGet_NotFound(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.Apply, error) {
			return nil, tfe.ErrResourceNotFound
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-missing",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for not found apply")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestAppliesErroredState_WritesToStdout(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	stateContent := `{"version": 4, "terraform_version": "1.0.0"}`

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "https://storage.example.com/state/errored-state.tfstate", nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(url string) ([]byte, error) {
			if url != "https://storage.example.com/state/errored-state.tfstate" {
				return nil, errors.New("unexpected URL")
			}
			return []byte(stateContent), nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout.String() != stateContent {
		t.Errorf("expected state content on stdout, got: %s", stdout.String())
	}
}

func TestAppliesErroredState_WritesToFile(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	stateContent := `{"version": 4, "state": "data"}`

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "https://storage.example.com/errored.tfstate", nil
		},
	}

	outFile := filepath.Join(t.TempDir(), "errored.tfstate")
	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-123",
		Out:           outFile,
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return []byte(stateContent), nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check file was written
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != stateContent {
		t.Errorf("expected state content in file, got: %s", string(content))
	}

	// Check meta JSON on stdout
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse meta JSON: %v", err)
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected 'meta' field in JSON output")
	}
	if meta["written_to"] != outFile {
		t.Errorf("expected written_to=%s, got %v", outFile, meta["written_to"])
	}
}

func TestAppliesErroredState_TableMode(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "https://storage.example.com/state.tfstate", nil
		},
	}

	outFile := filepath.Join(t.TempDir(), "state.tfstate")
	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-123",
		Out:           outFile,
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return []byte("state-data"), nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Errored state written to") {
		t.Error("expected success message in output")
	}
	if !strings.Contains(out, outFile) {
		t.Error("expected file path in output")
	}
}

func TestAppliesErroredState_NotAvailable(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("errored state not available (apply not found, no errored state uploaded, or unauthorized)")
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-missing",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when errored state not available")
	}
	if !strings.Contains(err.Error(), "errored state") {
		t.Errorf("expected 'errored state' in error, got: %v", err)
	}
}

func TestAppliesErroredState_DownloadError(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "https://storage.example.com/state.tfstate", nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return nil, errors.New("connection refused")
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error on download failure")
	}
	if !strings.Contains(err.Error(), "download errored state") {
		t.Errorf("expected 'download errored state' in error, got: %v", err)
	}
}

func TestAppliesGet_FailsWhenSettingsMissing(t *testing.T) {
	baseDir := t.TempDir() // Empty, no settings

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:          "apply-123",
		baseDir:     baseDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      &stdout,
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings missing")
	}
	if !strings.Contains(err.Error(), "tfccli init") {
		t.Errorf("expected 'tfccli init' suggestion in error, got: %v", err)
	}
}

func TestAppliesErroredState_NoAuthorizationForwarded(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	var downloadURLReceived string

	fakeClient := &fakeAppliesClient{
		GetErroredStateURLFn: func(_ context.Context, _ string) (string, error) {
			return "https://archivist.example.com/errored-state.tfstate?token=signed", nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesErroredStateCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(url string) ([]byte, error) {
			downloadURLReceived = url
			// This mock represents that the download happens without auth header
			// The actual implementation uses http.Client.Get which doesn't add auth
			return []byte("state-content"), nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the download URL was the redirect URL (not the original API URL)
	if downloadURLReceived != "https://archivist.example.com/errored-state.tfstate?token=signed" {
		t.Errorf("expected download from redirect URL, got: %s", downloadURLReceived)
	}
}

func TestApplies_ContextOverride(t *testing.T) {
	baseDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address: "default.terraform.io",
			},
			"prod": {
				Address: "prod.terraform.io",
			},
		},
	}
	if err := config.Save(settings, baseDir); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	fakeEnv := &appliesTestEnv{
		vars: map[string]string{
			"TF_TOKEN_prod_terraform_io": "test-token",
		},
	}
	fakeFS := &appliesTestFS{
		homeDir: baseDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var usedAddress string
	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.Apply, error) {
			return &tfe.Apply{ID: "apply-123", Status: tfe.ApplyFinished}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (appliesClient, error) {
			usedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Context: "prod", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usedAddress != "prod.terraform.io" {
		t.Errorf("expected prod.terraform.io, got %s", usedAddress)
	}
}

func TestApplies_AddressOverride(t *testing.T) {
	baseDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address: "app.terraform.io",
			},
		},
	}
	if err := config.Save(settings, baseDir); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Create resolver that provides token for the override address
	fakeEnv := &appliesTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_terraform_io": "test-token",
		},
	}
	fakeFS := &appliesTestFS{
		homeDir: baseDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var usedAddress string
	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.Apply, error) {
			return &tfe.Apply{ID: "apply-123", Status: tfe.ApplyFinished}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (appliesClient, error) {
			usedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Address: "custom.terraform.io", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usedAddress != "custom.terraform.io" {
		t.Errorf("expected custom.terraform.io, got %s", usedAddress)
	}
}

func TestAppliesGet_APIError(t *testing.T) {
	baseDir, resolver := setupAppliesTest(t)

	fakeClient := &fakeAppliesClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.Apply, error) {
			return nil, tfe.ErrUnauthorized
		},
	}

	var stdout bytes.Buffer
	cmd := &AppliesGetCmd{
		ID:            "apply-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (appliesClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("expected 'unauthorized' in error, got: %v", err)
	}
}
