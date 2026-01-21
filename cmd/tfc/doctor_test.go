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

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeEnv implements auth.EnvGetter for testing.
type fakeEnv struct {
	vars map[string]string
}

func (e *fakeEnv) Getenv(key string) string {
	return e.vars[key]
}

// fakeFS implements auth.FSReader for testing.
type fakeFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *fakeFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// fakeDoctorClient implements doctorClient for testing.
type fakeDoctorClient struct {
	pingErr error
}

func (c *fakeDoctorClient) Ping(_ context.Context) error {
	return c.pingErr
}

// captureStdout creates a temp file for capturing output.
func captureStdout(t *testing.T) (*os.File, func() string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cleanup := func() string {
		f.Seek(0, 0)
		var buf bytes.Buffer
		buf.ReadFrom(f)
		f.Close()
		return buf.String()
	}

	return f, cleanup
}

func TestDoctor_FailsWhenSettingsMissing(t *testing.T) {
	// Gherkin: Doctor fails when settings missing
	// Given no settings.json exists
	tmpDir := t.TempDir()

	stdout, getOutput := captureStdout(t)

	cmd := &DoctorCmd{
		baseDir:     tmpDir,
		stdout:      stdout,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "json"}

	// When I run "tfc doctor"
	err := cmd.Run(cli)

	// Then stderr contains "tfc init" (via error message in output)
	// And exit code is 2 (returned as runtime error)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	out := getOutput()
	if !strings.Contains(out, "tfc init") {
		t.Errorf("expected output to contain 'tfc init', got: %s", out)
	}

	// Verify it's a runtime error (exit code 2)
	var runtimeErr interface{ Error() string }
	if !errors.As(err, &runtimeErr) {
		t.Errorf("expected runtime error, got: %T", err)
	}
}

func TestDoctor_ReportsTokenSource(t *testing.T) {
	// Gherkin: Doctor reports token source
	// Given env token exists for host
	// And API connectivity is OK
	tmpDir := t.TempDir()

	// Create valid settings
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

	stdout, getOutput := captureStdout(t)

	// Create fake env with token
	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "fake-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "json"}

	// When I run "tfc doctor --output-format=json"
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then stdout.checks.token.status = "pass"
	// And stdout.checks.token.source = "env"
	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	tokenCheck := findCheck(result.Checks, "token")
	if tokenCheck == nil {
		t.Fatal("token check not found in output")
	}

	if tokenCheck.Status != "PASS" {
		t.Errorf("expected token status PASS, got: %s", tokenCheck.Status)
	}
	if !strings.Contains(tokenCheck.Detail, "env") {
		t.Errorf("expected token detail to contain 'env', got: %s", tokenCheck.Detail)
	}
}

func TestDoctor_FailsOnConnectivityError(t *testing.T) {
	// Gherkin: Doctor fails on connectivity error
	// Given token exists
	// And API responds 500
	tmpDir := t.TempDir()

	// Create valid settings
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

	stdout, getOutput := captureStdout(t)

	// Create fake env with token
	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "fake-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: errors.New("500 Internal Server Error")}, nil
		},
	}
	cli := &CLI{OutputFormat: "json"}

	// When I run "tfc doctor"
	err := cmd.Run(cli)

	// Then exit code is 2
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// And output indicates connectivity failure
	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	connCheck := findCheck(result.Checks, "connectivity")
	if connCheck == nil {
		t.Fatal("connectivity check not found in output")
	}

	if connCheck.Status != "FAIL" {
		t.Errorf("expected connectivity status FAIL, got: %s", connCheck.Status)
	}
}

func TestDoctor_TableOutput(t *testing.T) {
	// Test table output format
	tmpDir := t.TempDir()

	// Create valid settings
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

	stdout, getOutput := captureStdout(t)

	// Create fake env with token
	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "fake-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false}, // not TTY, so no styling
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "table"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := getOutput()

	// Verify table headers
	if !strings.Contains(out, "CHECK") {
		t.Errorf("expected table to contain 'CHECK' header, got: %s", out)
	}
	if !strings.Contains(out, "STATUS") {
		t.Errorf("expected table to contain 'STATUS' header, got: %s", out)
	}
	if !strings.Contains(out, "DETAIL") {
		t.Errorf("expected table to contain 'DETAIL' header, got: %s", out)
	}

	// Verify check names
	if !strings.Contains(out, "settings") {
		t.Errorf("expected table to contain 'settings' check, got: %s", out)
	}
	if !strings.Contains(out, "token") {
		t.Errorf("expected table to contain 'token' check, got: %s", out)
	}
	if !strings.Contains(out, "connectivity") {
		t.Errorf("expected table to contain 'connectivity' check, got: %s", out)
	}
}

func TestDoctor_ContextOverride(t *testing.T) {
	// Test --context flag override
	tmpDir := t.TempDir()

	// Create settings with multiple contexts
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "app.terraform.io",
				LogLevel: "info",
			},
			"prod": {
				Address:  "tfe.example.com",
				LogLevel: "warn",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	stdout, getOutput := captureStdout(t)

	// Create fake env with token for prod
	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_tfe_example_com": "prod-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "json", Context: "prod"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	// Verify context check shows prod
	ctxCheck := findCheck(result.Checks, "context")
	if ctxCheck == nil {
		t.Fatal("context check not found")
	}
	if !strings.Contains(ctxCheck.Detail, "prod") {
		t.Errorf("expected context detail to contain 'prod', got: %s", ctxCheck.Detail)
	}

	// Verify address check shows tfe.example.com
	addrCheck := findCheck(result.Checks, "address")
	if addrCheck == nil {
		t.Fatal("address check not found")
	}
	if !strings.Contains(addrCheck.Detail, "tfe.example.com") {
		t.Errorf("expected address detail to contain 'tfe.example.com', got: %s", addrCheck.Detail)
	}
}

func TestDoctor_AddressOverride(t *testing.T) {
	// Test --address flag override
	tmpDir := t.TempDir()

	// Create valid settings
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

	stdout, getOutput := captureStdout(t)

	// Create fake env with token for override address
	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_override_example_com": "override-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "json", Address: "override.example.com"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	// Verify address check shows override address
	addrCheck := findCheck(result.Checks, "address")
	if addrCheck == nil {
		t.Fatal("address check not found")
	}
	if !strings.Contains(addrCheck.Detail, "override.example.com") {
		t.Errorf("expected address detail to contain 'override.example.com', got: %s", addrCheck.Detail)
	}
}

func TestDoctor_TokenFromCredentialsFile(t *testing.T) {
	// Test token source from credentials.tfrc.json
	tmpDir := t.TempDir()

	// Create valid settings
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

	stdout, getOutput := captureStdout(t)

	// Create fake credentials file
	credPath := filepath.Join(tmpDir, ".terraform.d", "credentials.tfrc.json")
	fakeEnvMap := &fakeEnv{
		vars: make(map[string]string), // No env token
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files: map[string][]byte{
			credPath: []byte(`{"credentials":{"app.terraform.io":{"token":"file-token"}}}`),
		},
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	tokenCheck := findCheck(result.Checks, "token")
	if tokenCheck == nil {
		t.Fatal("token check not found")
	}

	if tokenCheck.Status != "PASS" {
		t.Errorf("expected token status PASS, got: %s", tokenCheck.Status)
	}
	if !strings.Contains(tokenCheck.Detail, "credentials.tfrc.json") {
		t.Errorf("expected token detail to contain 'credentials.tfrc.json', got: %s", tokenCheck.Detail)
	}
}

func TestDoctor_AllChecksPASS(t *testing.T) {
	// Test that all checks pass when everything is configured correctly
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
		t.Fatalf("failed to save settings: %v", err)
	}

	stdout, getOutput := captureStdout(t)

	fakeEnvMap := &fakeEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "fake-token",
		},
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
		clientFactory: func(_ tfcapi.ClientConfig) (doctorClient, error) {
			return &fakeDoctorClient{pingErr: nil}, nil
		},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("expected no error when all checks pass, got: %v", err)
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	// All checks should be PASS
	for _, check := range result.Checks {
		if check.Status != "PASS" {
			t.Errorf("expected check %q status PASS, got: %s", check.Name, check.Status)
		}
	}

	// Should have all expected checks
	expectedChecks := []string{"settings", "context", "address", "token", "connectivity"}
	for _, name := range expectedChecks {
		if findCheck(result.Checks, name) == nil {
			t.Errorf("missing expected check: %s", name)
		}
	}
}

func TestDoctor_NoTokenError(t *testing.T) {
	// Test that missing token produces actionable error
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
		t.Fatalf("failed to save settings: %v", err)
	}

	stdout, getOutput := captureStdout(t)

	// No token anywhere
	fakeEnvMap := &fakeEnv{
		vars: make(map[string]string),
	}
	fakeFSMap := &fakeFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	cmd := &DoctorCmd{
		baseDir:       tmpDir,
		stdout:        stdout,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		tokenResolver: &auth.TokenResolver{Env: fakeEnvMap, FS: fakeFSMap},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error when no token found")
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	tokenCheck := findCheck(result.Checks, "token")
	if tokenCheck == nil {
		t.Fatal("token check not found")
	}

	if tokenCheck.Status != "FAIL" {
		t.Errorf("expected token status FAIL, got: %s", tokenCheck.Status)
	}

	// Should suggest terraform login
	if !strings.Contains(tokenCheck.Detail, "terraform login") {
		t.Errorf("expected token detail to suggest 'terraform login', got: %s", tokenCheck.Detail)
	}
}

// TestDoctor_ContextNotFound tests error when --context flag specifies non-existent context.
func TestDoctor_ContextNotFound(t *testing.T) {
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
		t.Fatalf("failed to save settings: %v", err)
	}

	stdout, getOutput := captureStdout(t)

	cmd := &DoctorCmd{
		baseDir:     tmpDir,
		stdout:      stdout,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "json", Context: "nonexistent"}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	ctxCheck := findCheck(result.Checks, "context")
	if ctxCheck == nil {
		t.Fatal("context check not found")
	}

	if ctxCheck.Status != "FAIL" {
		t.Errorf("expected context status FAIL, got: %s", ctxCheck.Status)
	}
	if !strings.Contains(ctxCheck.Detail, "nonexistent") {
		t.Errorf("expected detail to contain 'nonexistent', got: %s", ctxCheck.Detail)
	}
}

// TestDoctor_InvalidAddressFormat tests error when address is malformed.
func TestDoctor_InvalidAddressFormat(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings with invalid address
	settings := &config.Settings{
		CurrentContext: "default",
		Contexts: map[string]config.Context{
			"default": {
				Address:  "://invalid-url", // Malformed URL
				LogLevel: "info",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	stdout, getOutput := captureStdout(t)

	cmd := &DoctorCmd{
		baseDir:     tmpDir,
		stdout:      stdout,
		ttyDetector: &output.FakeTTYDetector{IsTTYValue: false},
	}
	cli := &CLI{OutputFormat: "json"}

	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	out := getOutput()

	var result DoctorResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}

	addrCheck := findCheck(result.Checks, "address")
	if addrCheck == nil {
		t.Fatal("address check not found")
	}

	if addrCheck.Status != "FAIL" {
		t.Errorf("expected address status FAIL, got: %s", addrCheck.Status)
	}
	if !strings.Contains(addrCheck.Detail, "invalid address") {
		t.Errorf("expected detail to contain 'invalid address', got: %s", addrCheck.Detail)
	}
}

// findCheck finds a check by name in the results.
func findCheck(checks []DoctorCheck, name string) *DoctorCheck {
	for i := range checks {
		if checks[i].Name == name {
			return &checks[i]
		}
	}
	return nil
}
