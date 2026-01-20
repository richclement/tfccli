package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
	"github.com/richclement/tfccli/internal/ui"
)

// fakeCVClient is a mock for testing.
type fakeCVClient struct {
	ListFunc     func(ctx context.Context, workspaceID string, opts *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error)
	ReadFunc     func(ctx context.Context, cvID string) (*tfe.ConfigurationVersion, error)
	CreateFunc   func(ctx context.Context, workspaceID string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error)
	UploadFunc   func(ctx context.Context, uploadURL string, reader io.Reader) error
	DownloadFunc func(ctx context.Context, cvID string) ([]byte, error)
	ArchiveFunc  func(ctx context.Context, cvID string) error
}

func (f *fakeCVClient) List(ctx context.Context, workspaceID string, opts *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
	if f.ListFunc != nil {
		return f.ListFunc(ctx, workspaceID, opts)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeCVClient) Read(ctx context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
	if f.ReadFunc != nil {
		return f.ReadFunc(ctx, cvID)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeCVClient) Create(ctx context.Context, workspaceID string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
	if f.CreateFunc != nil {
		return f.CreateFunc(ctx, workspaceID, opts)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeCVClient) Upload(ctx context.Context, uploadURL string, reader io.Reader) error {
	if f.UploadFunc != nil {
		return f.UploadFunc(ctx, uploadURL, reader)
	}
	return errors.New("not implemented")
}

func (f *fakeCVClient) Download(ctx context.Context, cvID string) ([]byte, error) {
	if f.DownloadFunc != nil {
		return f.DownloadFunc(ctx, cvID)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeCVClient) Archive(ctx context.Context, cvID string) error {
	if f.ArchiveFunc != nil {
		return f.ArchiveFunc(ctx, cvID)
	}
	return errors.New("not implemented")
}

// cvTestEnv implements auth.EnvGetter for testing.
type cvTestEnv struct {
	vars map[string]string
}

func (e *cvTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// cvTestFS implements auth.FSReader for testing.
type cvTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *cvTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *cvTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

func setupCVTest(t *testing.T) (string, *auth.TokenResolver) {
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

	fakeEnv := &cvTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFS := &cvTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}
	return tmpDir, resolver
}

func TestCVList_JSON(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ListFunc: func(_ context.Context, workspaceID string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
			if workspaceID != "ws-123" {
				return nil, errors.New("workspace not found")
			}
			return []*tfe.ConfigurationVersion{
				{ID: "cv-1", Status: tfe.ConfigurationUploaded, Source: tfe.ConfigurationSourceTerraform, AutoQueueRuns: true, Speculative: false},
				{ID: "cv-2", Status: tfe.ConfigurationPending, Source: tfe.ConfigurationSourceAPI, AutoQueueRuns: false, Speculative: true},
			}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
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
		t.Fatal("expected 'data' field to be array in JSON output")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 items, got %d", len(data))
	}
}

func TestCVList_Table(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ListFunc: func(_ context.Context, _ string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
			return []*tfe.ConfigurationVersion{
				{ID: "cv-1", Status: tfe.ConfigurationUploaded, Source: tfe.ConfigurationSourceTerraform, AutoQueueRuns: true, Speculative: false},
			}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
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
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") {
		t.Error("expected table headers ID, STATUS")
	}
	if !strings.Contains(out, "cv-1") {
		t.Error("expected cv-1 in output")
	}
}

func TestCVGet_JSON(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
			if cvID != "cv-123" {
				return nil, errors.New("not found")
			}
			return &tfe.ConfigurationVersion{
				ID:            "cv-123",
				Status:        tfe.ConfigurationUploaded,
				Source:        tfe.ConfigurationSourceTerraform,
				AutoQueueRuns: true,
				Speculative:   false,
				UploadURL:     "https://example.com/upload",
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
	if data["id"] != "cv-123" {
		t.Errorf("expected id=cv-123, got %v", data["id"])
	}
}

func TestCVCreate_JSON(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	var createdOpts tfe.ConfigurationVersionCreateOptions
	fakeClient := &fakeCVClient{
		CreateFunc: func(_ context.Context, workspaceID string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
			if workspaceID != "ws-123" {
				return nil, errors.New("workspace not found")
			}
			createdOpts = opts
			return &tfe.ConfigurationVersion{
				ID:            "cv-new",
				Status:        tfe.ConfigurationPending,
				Source:        tfe.ConfigurationSourceAPI,
				AutoQueueRuns: opts.AutoQueueRuns != nil && *opts.AutoQueueRuns,
				Speculative:   opts.Speculative != nil && *opts.Speculative,
				UploadURL:     "https://example.com/upload/cv-new",
			}, nil
		},
	}

	var stdout bytes.Buffer
	autoQueueRuns := true
	cmd := &CVCreateCmd{
		WorkspaceID:   "ws-123",
		AutoQueueRuns: &autoQueueRuns,
		Speculative:   false,
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

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected 'data' field in JSON output")
	}
	if data["id"] != "cv-new" {
		t.Errorf("expected id=cv-new, got %v", data["id"])
	}

	// Verify options were passed
	if createdOpts.AutoQueueRuns == nil || !*createdOpts.AutoQueueRuns {
		t.Error("expected auto_queue_runs to be true")
	}
}

func TestCVCreate_WithSpeculative(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	var createdOpts tfe.ConfigurationVersionCreateOptions
	fakeClient := &fakeCVClient{
		CreateFunc: func(_ context.Context, _ string, opts tfe.ConfigurationVersionCreateOptions) (*tfe.ConfigurationVersion, error) {
			createdOpts = opts
			return &tfe.ConfigurationVersion{
				ID:          "cv-spec",
				Status:      tfe.ConfigurationPending,
				Speculative: true,
			}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVCreateCmd{
		WorkspaceID:   "ws-123",
		Speculative:   true,
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

	if createdOpts.Speculative == nil || !*createdOpts.Speculative {
		t.Error("expected speculative to be true")
	}
}

func TestCVUpload_JSON(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	var uploadedURL string
	var uploadedContent []byte
	fakeClient := &fakeCVClient{
		ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
			return &tfe.ConfigurationVersion{
				ID:        cvID,
				Status:    tfe.ConfigurationPending,
				UploadURL: "https://archivist.example.com/upload/cv-123",
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
			return []byte("fake-tar-gz-content"), nil
		},
		uploadClient: func(url string, content []byte) error {
			uploadedURL = url
			uploadedContent = content
			return nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if uploadedURL != "https://archivist.example.com/upload/cv-123" {
		t.Errorf("expected upload URL https://archivist.example.com/upload/cv-123, got %s", uploadedURL)
	}
	if string(uploadedContent) != "fake-tar-gz-content" {
		t.Errorf("expected content 'fake-tar-gz-content', got %s", string(uploadedContent))
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected 'meta' field in JSON output")
	}
	if meta["status"] != "uploaded" {
		t.Errorf("expected status=uploaded, got %v", meta["status"])
	}
}

func TestCVUpload_NoUploadURL(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ReadFunc: func(_ context.Context, cvID string) (*tfe.ConfigurationVersion, error) {
			return &tfe.ConfigurationVersion{
				ID:        cvID,
				Status:    tfe.ConfigurationUploaded,
				UploadURL: "", // No upload URL
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
			return []byte("fake-tar-gz-content"), nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when no upload URL")
	}
	if !strings.Contains(err.Error(), "no upload URL") {
		t.Errorf("expected error about no upload URL, got: %v", err)
	}
}

func TestCVDownload_WritesToStdout(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		DownloadFunc: func(_ context.Context, cvID string) ([]byte, error) {
			return []byte("downloaded-config-content"), nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVDownloadCmd{
		ID:            "cv-123",
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

	if stdout.String() != "downloaded-config-content" {
		t.Errorf("expected 'downloaded-config-content', got %s", stdout.String())
	}
}

func TestCVDownload_WritesToFile(t *testing.T) {
	baseDir, resolver := setupCVTest(t)
	outPath := filepath.Join(t.TempDir(), "downloaded.tar.gz")

	fakeClient := &fakeCVClient{
		DownloadFunc: func(_ context.Context, cvID string) ([]byte, error) {
			return []byte("downloaded-config-content"), nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVDownloadCmd{
		ID:            "cv-123",
		Out:           outPath,
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

	// Verify file was written
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != "downloaded-config-content" {
		t.Errorf("expected 'downloaded-config-content', got %s", string(content))
	}

	// Verify meta output
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected 'meta' field in JSON output")
	}
	if meta["written_to"] != outPath {
		t.Errorf("expected written_to=%s, got %v", outPath, meta["written_to"])
	}
}

func TestCVArchive_PromptsWithoutForce(t *testing.T) {
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
		prompter: ui.NewScriptedPrompter().OnConfirm("Archive configuration version cv-123?", false), // Answer "no"
	}

	cli := &CLI{Force: false, OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if archiveCalled {
		t.Error("expected archive NOT to be called when user says no")
	}
	if !strings.Contains(stdout.String(), "Aborting") {
		t.Error("expected abort message in output")
	}
}

func TestCVArchive_WithForce(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	archiveCalled := false
	fakeClient := &fakeCVClient{
		ArchiveFunc: func(_ context.Context, cvID string) error {
			archiveCalled = true
			if cvID != "cv-123" {
				return errors.New("not found")
			}
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
		prompter: nil, // Should not be called with --force
	}

	cli := &CLI{Force: true, OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !archiveCalled {
		t.Error("expected archive to be called with --force")
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	meta, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected 'meta' field in JSON output")
	}
	if meta["status"] != "archived" {
		t.Errorf("expected status=archived, got %v", meta["status"])
	}
}

func TestCVArchive_ConfirmYes(t *testing.T) {
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
		prompter: ui.NewScriptedPrompter().OnConfirm("Archive configuration version cv-123?", true), // Answer "yes"
	}

	cli := &CLI{Force: false, OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !archiveCalled {
		t.Error("expected archive to be called when user confirms")
	}
}

func TestCVList_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir() // Empty dir, no settings.json

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: nil,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
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

func TestCVList_APIError(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ListFunc: func(_ context.Context, _ string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
			return nil, errors.New("API error: workspace not found")
		},
	}

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
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
		t.Errorf("expected workspace not found error, got: %v", err)
	}
}

func TestCVGet_NotFound(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	fakeClient := &fakeCVClient{
		ReadFunc: func(_ context.Context, _ string) (*tfe.ConfigurationVersion, error) {
			return nil, errors.New("not found")
		},
	}

	var stdout bytes.Buffer
	cmd := &CVGetCmd{
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
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestCV_ContextOverride(t *testing.T) {
	tmpDir := t.TempDir()

	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:    "default.terraform.io",
				DefaultOrg: "default-org",
				LogLevel:   "info",
			},
			"prod": {
				Address:    "prod.terraform.io",
				DefaultOrg: "prod-org",
				LogLevel:   "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatal(err)
	}

	fakeEnv := &cvTestEnv{
		vars: map[string]string{
			"TF_TOKEN_prod_terraform_io": "prod-token",
		},
	}
	fakeFS := &cvTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	fakeClient := &fakeCVClient{
		ListFunc: func(_ context.Context, _ string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
			return []*tfe.ConfigurationVersion{}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{Context: "prod", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCV_AddressOverride(t *testing.T) {
	baseDir, resolver := setupCVTest(t)

	// Also set token for overridden host
	fakeEnv := &cvTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_terraform_io": "custom-token",
		},
	}
	fakeFS := &cvTestFS{
		homeDir: baseDir,
		files:   make(map[string][]byte),
	}
	resolver = &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	fakeClient := &fakeCVClient{
		ListFunc: func(_ context.Context, _ string, _ *tfe.ConfigurationVersionListOptions) ([]*tfe.ConfigurationVersion, error) {
			return []*tfe.ConfigurationVersion{}, nil
		},
	}

	var stdout bytes.Buffer
	cmd := &CVListCmd{
		WorkspaceID:   "ws-123",
		baseDir:       baseDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (cvClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{Address: "custom.terraform.io", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
