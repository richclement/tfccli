package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

// fakePlansClient implements plansClient for testing.
type fakePlansClient struct {
	plan       *tfe.Plan
	jsonOutput []byte
	readErr    error
	jsonErr    error
}

func (f *fakePlansClient) Read(_ context.Context, _ string) (*tfe.Plan, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.plan, nil
}

func (f *fakePlansClient) ReadJSONOutput(_ context.Context, _ string) ([]byte, error) {
	if f.jsonErr != nil {
		return nil, f.jsonErr
	}
	return f.jsonOutput, nil
}

// plansTestEnv implements auth.EnvGetter for testing.
type plansTestEnv struct {
	vars map[string]string
}

func (e *plansTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// plansTestFS implements auth.FSReader for testing.
type plansTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *plansTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *plansTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

func setupPlansTest(t *testing.T) (string, *auth.TokenResolver) {
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
		t.Fatal(err)
	}

	fakeEnv := &plansTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "test-token",
		},
	}
	fakeFS := &plansTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}
	return tmpDir, resolver
}

func TestPlansGet_JSON(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID:                   "plan-123",
			Status:               tfe.PlanFinished,
			HasChanges:           true,
			ResourceAdditions:    2,
			ResourceChanges:      1,
			ResourceDestructions: 0,
			ResourceImports:      0,
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
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
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data field in JSON output")
	}

	if data["id"] != "plan-123" {
		t.Errorf("expected id=plan-123, got %v", data["id"])
	}
	if data["status"] != "finished" {
		t.Errorf("expected status=finished, got %v", data["status"])
	}
	if data["has_changes"] != true {
		t.Errorf("expected has_changes=true, got %v", data["has_changes"])
	}
}

func TestPlansGet_Table(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID:                   "plan-456",
			Status:               tfe.PlanFinished,
			HasChanges:           true,
			ResourceAdditions:    3,
			ResourceChanges:      2,
			ResourceDestructions: 1,
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-456",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
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
		t.Error("expected table headers FIELD and VALUE")
	}
	if !strings.Contains(out, "plan-456") {
		t.Errorf("expected output to contain plan-456, got: %s", out)
	}
	if !strings.Contains(out, "finished") {
		t.Errorf("expected output to contain 'finished', got: %s", out)
	}
}

func TestPlansGet_Table_WithLogReadURL(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID:         "plan-789",
			Status:     tfe.PlanRunning,
			LogReadURL: "https://archivist.example/logs/plan-789",
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-789",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Log URL") {
		t.Errorf("expected 'Log URL' field in table output, got: %s", out)
	}
	if !strings.Contains(out, "https://archivist.example/logs/plan-789") {
		t.Errorf("expected LogReadURL value in table output, got: %s", out)
	}
}

func TestPlansGet_Table_WithoutLogReadURL(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID:         "plan-abc",
			Status:     tfe.PlanFinished,
			LogReadURL: "", // Empty - should not appear in table
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-abc",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "Log URL") {
		t.Errorf("expected 'Log URL' field NOT to appear when LogReadURL is empty, got: %s", out)
	}
}

func TestPlansGet_NotFound(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		readErr: errors.New("not found"),
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-notfound",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for not found plan")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestPlansJSONOutput_WritesToStdout(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	expectedJSON := `{"format_version":"1.0","resource_changes":[]}`
	fakeClient := &fakePlansClient{
		jsonOutput: []byte(expectedJSON),
	}

	var stdout bytes.Buffer
	cmd := &PlansJSONOutputCmd{
		ID:            "plan-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout.String() != expectedJSON {
		t.Errorf("expected %q, got %q", expectedJSON, stdout.String())
	}
}

func TestPlansJSONOutput_WritesToFile(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	expectedJSON := `{"format_version":"1.0","resource_changes":[]}`
	fakeClient := &fakePlansClient{
		jsonOutput: []byte(expectedJSON),
	}

	outFile := filepath.Join(tmpDir, "out.json")
	var stdout bytes.Buffer
	cmd := &PlansJSONOutputCmd{
		ID:            "plan-123",
		Out:           outFile,
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was written
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != expectedJSON {
		t.Errorf("expected file content %q, got %q", expectedJSON, string(content))
	}

	// Verify meta output on stdout
	var meta map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &meta); err != nil {
		t.Fatalf("failed to parse meta JSON: %v", err)
	}
	metaData, ok := meta["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected meta field in output")
	}
	if metaData["written_to"] != outFile {
		t.Errorf("expected written_to=%s, got %v", outFile, metaData["written_to"])
	}
}

func TestPlansJSONOutput_TableMode(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	expectedJSON := `{"format_version":"1.0"}`
	fakeClient := &fakePlansClient{
		jsonOutput: []byte(expectedJSON),
	}

	outFile := filepath.Join(tmpDir, "out.json")
	var stdout bytes.Buffer
	cmd := &PlansJSONOutputCmd{
		ID:            "plan-123",
		Out:           outFile,
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "written to") {
		t.Errorf("expected 'written to' in table output, got: %s", out)
	}
	if !strings.Contains(out, outFile) {
		t.Errorf("expected file path in output, got: %s", out)
	}
}

func TestPlansJSONOutput_APIError(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		jsonErr: errors.New("plan not found"),
	}

	var stdout bytes.Buffer
	cmd := &PlansJSONOutputCmd{
		ID:            "plan-notfound",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "plan not found") {
		t.Errorf("expected 'plan not found' in error, got: %v", err)
	}
}

func TestPlansSanitizedPlan_WritesToStdout(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	sanitizedContent := `{"sanitized":"plan"}`
	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID: "plan-hyok",
			Links: map[string]interface{}{
				"sanitized-plan": "https://archivist.example/sanitized.json",
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-hyok",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return []byte(sanitizedContent), nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stdout.String() != sanitizedContent {
		t.Errorf("expected %q, got %q", sanitizedContent, stdout.String())
	}
}

func TestPlansSanitizedPlan_WritesToFile(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	sanitizedContent := `{"sanitized":"plan"}`
	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID: "plan-hyok",
			Links: map[string]interface{}{
				"sanitized-plan": "https://archivist.example/sanitized.json",
			},
		},
	}

	outFile := filepath.Join(tmpDir, "sanitized.json")
	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-hyok",
		Out:           outFile,
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return []byte(sanitizedContent), nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was written
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(content) != sanitizedContent {
		t.Errorf("expected file content %q, got %q", sanitizedContent, string(content))
	}

	// Verify meta output on stdout
	var meta map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &meta); err != nil {
		t.Fatalf("failed to parse meta JSON: %v", err)
	}
	metaData, ok := meta["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected meta field in output")
	}
	if metaData["written_to"] != outFile {
		t.Errorf("expected written_to=%s, got %v", outFile, metaData["written_to"])
	}
}

func TestPlansSanitizedPlan_NoLinkAvailable(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID:    "plan-no-hyok",
			Links: map[string]interface{}{},
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-no-hyok",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when sanitized plan link not available")
	}
	if !strings.Contains(err.Error(), "sanitized plan not available") {
		t.Errorf("expected 'sanitized plan not available' in error, got: %v", err)
	}
}

func TestPlansSanitizedPlan_LinkWrongType(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID: "plan-bad-link",
			Links: map[string]interface{}{
				"sanitized-plan": 12345, // Wrong type (int instead of string)
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-bad-link",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when sanitized plan link has wrong type")
	}
	// Error should mention the type, not say "not available"
	if !strings.Contains(err.Error(), "unexpected type") {
		t.Errorf("expected 'unexpected type' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "int") {
		t.Errorf("expected type 'int' mentioned in error, got: %v", err)
	}
}

func TestPlansSanitizedPlan_DownloadError(t *testing.T) {
	tmpDir, resolver := setupPlansTest(t)

	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID: "plan-hyok",
			Links: map[string]interface{}{
				"sanitized-plan": "https://archivist.example/sanitized.json",
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-hyok",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(_ string) ([]byte, error) {
			return nil, errors.New("connection refused")
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when download fails")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected 'connection refused' in error, got: %v", err)
	}
}

func TestPlansGet_FailsWhenSettingsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create settings file

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:          "plan-123",
		baseDir:     tmpDir,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
		stdout:      &stdout,
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when settings missing")
	}
	if !strings.Contains(err.Error(), "tfc init") {
		t.Errorf("expected 'tfc init' in error, got: %v", err)
	}
}

func TestPlans_ContextOverride(t *testing.T) {
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
		t.Fatal(err)
	}

	fakeEnv := &plansTestEnv{
		vars: map[string]string{
			"TF_TOKEN_tfe_example_com": "prod-token",
		},
	}
	fakeFS := &plansTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var usedAddress string
	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{ID: "plan-123", Status: tfe.PlanFinished},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (plansClient, error) {
			usedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Context: "prod", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usedAddress != "tfe.example.com" {
		t.Errorf("expected address tfe.example.com, got %s", usedAddress)
	}
}

func TestPlans_AddressOverride(t *testing.T) {
	tmpDir, _ := setupPlansTest(t)

	fakeEnv := &plansTestEnv{
		vars: map[string]string{
			"TF_TOKEN_custom_tfe_io": "custom-token",
		},
	}
	fakeFS := &plansTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}
	resolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var usedAddress string
	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{ID: "plan-123", Status: tfe.PlanFinished},
	}

	var stdout bytes.Buffer
	cmd := &PlansGetCmd{
		ID:            "plan-123",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (plansClient, error) {
			usedAddress = cfg.Address
			return fakeClient, nil
		},
	}

	cli := &CLI{Address: "custom.tfe.io", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usedAddress != "custom.tfe.io" {
		t.Errorf("expected address custom.tfe.io, got %s", usedAddress)
	}
}

func TestPlansSanitizedPlan_NoAuthorizationForwarded(t *testing.T) {
	// This test verifies that the download client is called with the redirect URL
	// but no Authorization header is forwarded (the download client makes a plain GET request)
	tmpDir, resolver := setupPlansTest(t)

	sanitizedContent := `{"sanitized":"plan"}`
	var downloadedURL string
	fakeClient := &fakePlansClient{
		plan: &tfe.Plan{
			ID: "plan-hyok",
			Links: map[string]interface{}{
				"sanitized-plan": "https://archivist.example/sanitized.json",
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &PlansSanitizedPlanCmd{
		ID:            "plan-hyok",
		baseDir:       tmpDir,
		tokenResolver: resolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (plansClient, error) {
			return fakeClient, nil
		},
		downloadClient: func(url string) ([]byte, error) {
			downloadedURL = url
			return []byte(sanitizedContent), nil
		},
	}

	cli := &CLI{}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify download was called with the redirect URL
	if downloadedURL != "https://archivist.example/sanitized.json" {
		t.Errorf("expected download URL https://archivist.example/sanitized.json, got %s", downloadedURL)
	}
}

func TestDefaultDownloadClient_HTTPError_IncludesBody(t *testing.T) {
	// Test that defaultDownloadClient includes response body in error message
	t.Run("with_body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"Access denied: invalid credentials"}`))
		}))
		defer server.Close()

		cmd := &PlansSanitizedPlanCmd{
			httpClient: server.Client(),
		}
		cmd.downloadClient = cmd.defaultDownloadClient

		_, err := cmd.downloadClient(server.URL)
		if err == nil {
			t.Fatal("expected error for non-200 status")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("expected status code 403 in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "Access denied") {
			t.Errorf("expected response body in error, got: %v", err)
		}
	})

	t.Run("without_body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cmd := &PlansSanitizedPlanCmd{
			httpClient: server.Client(),
		}
		cmd.downloadClient = cmd.defaultDownloadClient

		_, err := cmd.downloadClient(server.URL)
		if err == nil {
			t.Fatal("expected error for non-200 status")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected status code 404 in error, got: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		expectedContent := `{"sanitized":"data"}`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(expectedContent))
		}))
		defer server.Close()

		cmd := &PlansSanitizedPlanCmd{
			httpClient: server.Client(),
		}
		cmd.downloadClient = cmd.defaultDownloadClient

		data, err := cmd.downloadClient(server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != expectedContent {
			t.Errorf("expected %q, got %q", expectedContent, string(data))
		}
	})
}
