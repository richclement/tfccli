package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/richclement/tfccli/internal/auth"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// fakeUsersClient is a mock usersClient for testing.
type fakeUsersClient struct {
	user *UserResponse
	err  error
}

func (c *fakeUsersClient) Read(_ context.Context, _ string) (*UserResponse, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.user, nil
}

// usersTestEnv implements auth.EnvGetter for testing.
type usersTestEnv struct {
	vars map[string]string
}

func (e *usersTestEnv) Getenv(key string) string {
	return e.vars[key]
}

// usersTestFS implements auth.FSReader for testing.
type usersTestFS struct {
	files   map[string][]byte
	homeDir string
}

func (f *usersTestFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (f *usersTestFS) UserHomeDir() (string, error) {
	return f.homeDir, nil
}

// setupUsersTestSettings creates test settings and returns the temp directory and token resolver.
func setupUsersTestSettings(t *testing.T) (string, *auth.TokenResolver) {
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
			"prod": {
				Address:    "tfe.example.com",
				DefaultOrg: "prod-org",
				LogLevel:   "warn",
			},
		},
	}
	if err := config.Save(settings, tmpDir); err != nil {
		t.Fatalf("failed to save test settings: %v", err)
	}

	// Create fake env with token
	fakeEnv := &usersTestEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io":   "test-token",
			"TF_TOKEN_tfe_example_com":    "prod-token",
			"TF_TOKEN_custom_example_com": "custom-token",
		},
	}
	fakeFS := &usersTestFS{
		homeDir: tmpDir,
		files:   make(map[string][]byte),
	}

	tokenResolver := &auth.TokenResolver{
		Env: fakeEnv,
		FS:  fakeFS,
	}

	return tmpDir, tokenResolver
}

func TestUsersGet_JSON(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	user := &UserResponse{
		Data: UserData{
			ID:   "user-abc123",
			Type: "users",
			Attributes: UserAttributes{
				Username:         "testuser",
				Email:            "test@example.com",
				AvatarURL:        "https://example.com/avatar.png",
				IsServiceAccount: false,
				V2Only:           true,
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-abc123",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{user: user}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse JSON output
	var result UserResponse
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if result.Data.ID != "user-abc123" {
		t.Errorf("expected user ID 'user-abc123', got %q", result.Data.ID)
	}
	if result.Data.Attributes.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", result.Data.Attributes.Username)
	}
	if result.Data.Attributes.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", result.Data.Attributes.Email)
	}
}

func TestUsersGet_Table(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	user := &UserResponse{
		Data: UserData{
			ID:   "user-abc123",
			Type: "users",
			Attributes: UserAttributes{
				Username:         "testuser",
				Email:            "test@example.com",
				AvatarURL:        "https://example.com/avatar.png",
				IsServiceAccount: false,
			},
		},
	}

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-abc123",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: true},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{user: user}, nil
		},
	}

	cli := &CLI{OutputFormat: "table"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	// Check table contains expected headers and values
	if !strings.Contains(output, "FIELD") || !strings.Contains(output, "VALUE") {
		t.Errorf("expected table headers, got: %s", output)
	}
	if !strings.Contains(output, "ID") || !strings.Contains(output, "user-abc123") {
		t.Errorf("expected user ID in table, got: %s", output)
	}
	if !strings.Contains(output, "Username") || !strings.Contains(output, "testuser") {
		t.Errorf("expected username in table, got: %s", output)
	}
	if !strings.Contains(output, "Email") || !strings.Contains(output, "test@example.com") {
		t.Errorf("expected email in table, got: %s", output)
	}
}

func TestUsersGet_NotFound(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-404",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{
				err: &usersAPIError{message: "user not found: user-404"},
			}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(strings.ToLower(errStr), "not found") {
		t.Errorf("expected 'not found' in error, got: %s", errStr)
	}
}

func TestUsersGet_FailsWhenSettingsMissing(t *testing.T) {
	// Empty temp dir (no settings.json)
	baseDir := t.TempDir()

	fakeEnv := &usersTestEnv{vars: make(map[string]string)}
	fakeFS := &usersTestFS{homeDir: baseDir, files: make(map[string][]byte)}
	tokenResolver := &auth.TokenResolver{Env: fakeEnv, FS: fakeFS}

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-123",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for missing settings, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "tfc init") {
		t.Errorf("expected 'tfc init' suggestion in error, got: %s", errStr)
	}
}

func TestUsersGet_ContextOverride(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	user := &UserResponse{
		Data: UserData{
			ID:   "user-prod",
			Type: "users",
			Attributes: UserAttributes{
				Username: "produser",
			},
		},
	}

	var capturedAddress string
	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-prod",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (usersClient, error) {
			capturedAddress = cfg.Address
			return &fakeUsersClient{user: user}, nil
		},
	}

	cli := &CLI{Context: "prod", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAddress != "tfe.example.com" {
		t.Errorf("expected address 'tfe.example.com', got %q", capturedAddress)
	}
}

func TestUsersGet_AddressOverride(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	user := &UserResponse{
		Data: UserData{
			ID:   "user-custom",
			Type: "users",
			Attributes: UserAttributes{
				Username: "customuser",
			},
		},
	}

	var capturedAddress string
	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-custom",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(cfg tfcapi.ClientConfig) (usersClient, error) {
			capturedAddress = cfg.Address
			return &fakeUsersClient{user: user}, nil
		},
	}

	cli := &CLI{Address: "custom.example.com", OutputFormat: "json"}
	err := cmd.Run(cli)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAddress != "custom.example.com" {
		t.Errorf("expected address 'custom.example.com', got %q", capturedAddress)
	}
}

func TestUsersGet_APIError(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-err",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{
				err: &usersAPIError{message: "Internal Server Error"},
			}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "Internal Server Error") {
		t.Errorf("expected 'Internal Server Error' in error, got: %s", errStr)
	}
}

func TestUsersGet_UnauthorizedError(t *testing.T) {
	baseDir, tokenResolver := setupUsersTestSettings(t)

	var stdout bytes.Buffer
	cmd := &UsersGetCmd{
		UserID:        "user-401",
		baseDir:       baseDir,
		tokenResolver: tokenResolver,
		ttyDetector:   &output.FakeTTYDetector{IsTTYValue: false},
		stdout:        &stdout,
		clientFactory: func(_ tfcapi.ClientConfig) (usersClient, error) {
			return &fakeUsersClient{
				err: &usersAPIError{message: "unauthorized: invalid or missing API token"},
			}, nil
		},
	}

	cli := &CLI{OutputFormat: "json"}
	err := cmd.Run(cli)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(strings.ToLower(errStr), "unauthorized") {
		t.Errorf("expected 'unauthorized' in error, got: %s", errStr)
	}
}

// usersAPIError for testing error handling.
type usersAPIError struct {
	message string
}

func (e *usersAPIError) Error() string {
	return e.message
}
