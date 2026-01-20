package auth

import (
	"errors"
	"strings"
	"testing"
)

// mockEnv implements EnvGetter for testing.
type mockEnv struct {
	vars map[string]string
}

func (m *mockEnv) Getenv(key string) string {
	if m.vars == nil {
		return ""
	}
	return m.vars[key]
}

// mockFS implements FSReader for testing.
type mockFS struct {
	files   map[string]string
	homeDir string
}

func (m *mockFS) ReadFile(path string) ([]byte, error) {
	if m.files == nil {
		return nil, errors.New("file not found")
	}
	if content, ok := m.files[path]; ok {
		return []byte(content), nil
	}
	return nil, errors.New("file not found")
}

func (m *mockFS) UserHomeDir() (string, error) {
	if m.homeDir == "" {
		return "/home/testuser", nil
	}
	return m.homeDir, nil
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
		wantErr bool
	}{
		{
			name:    "bare hostname",
			address: "app.terraform.io",
			want:    "app.terraform.io",
		},
		{
			name:    "hostname with https scheme",
			address: "https://app.terraform.io",
			want:    "app.terraform.io",
		},
		{
			name:    "hostname with path",
			address: "app.terraform.io/eu",
			want:    "app.terraform.io",
		},
		{
			name:    "full URL with path",
			address: "https://tfe.example.com/v2/api",
			want:    "tfe.example.com",
		},
		{
			name:    "URL with port",
			address: "https://tfe.example.com:8443",
			want:    "tfe.example.com",
		},
		{
			name:    "empty address",
			address: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractHostname(tt.address)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractHostname() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ExtractHostname() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractHostname() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeHost(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"app.terraform.io", "app_terraform_io"},
		{"tfe.example.com", "tfe_example_com"},
		{"my-tfe.corp.net", "my_tfe_corp_net"},
		{"localhost", "localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := SanitizeHost(tt.hostname)
			if got != tt.want {
				t.Errorf("SanitizeHost(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
		})
	}
}

// Gherkin: Env token wins
func TestResolveToken_EnvWins(t *testing.T) {
	env := &mockEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "env-token",
		},
	}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/home/testuser/.terraform.d/credentials.tfrc.json": `{
				"credentials": {
					"app.terraform.io": {
						"token": "file-token"
					}
				}
			}`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "env-token" {
		t.Errorf("Token = %q, want %q", result.Token, "env-token")
	}
	if result.Source != SourceEnv {
		t.Errorf("Source = %q, want %q", result.Source, SourceEnv)
	}
}

// Gherkin: terraform login file token used when env missing
func TestResolveToken_CredentialsFileWhenEnvMissing(t *testing.T) {
	env := &mockEnv{
		vars: map[string]string{}, // no env token
	}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/home/testuser/.terraform.d/credentials.tfrc.json": `{
				"credentials": {
					"app.terraform.io": {
						"token": "file-token"
					}
				}
			}`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "file-token" {
		t.Errorf("Token = %q, want %q", result.Token, "file-token")
	}
	if result.Source != SourceCredentialsFile {
		t.Errorf("Source = %q, want %q", result.Source, SourceCredentialsFile)
	}
}

// Gherkin: Missing token yields actionable error
func TestResolveToken_NoTokenError(t *testing.T) {
	env := &mockEnv{vars: map[string]string{}}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files:   map[string]string{}, // no credentials file
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")

	if result != nil {
		t.Errorf("ResolveToken() result = %v, want nil", result)
	}
	if err == nil {
		t.Fatal("ResolveToken() error = nil, want error")
	}

	// Check it's a NoTokenError
	var noTokenErr *NoTokenError
	if !errors.As(err, &noTokenErr) {
		t.Fatalf("error type = %T, want *NoTokenError", err)
	}

	// Check error message contains actionable guidance
	errMsg := err.Error()
	if !strings.Contains(errMsg, "no API token found") {
		t.Errorf("error should contain 'no API token found', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "terraform login") {
		t.Errorf("error should suggest 'terraform login', got: %s", errMsg)
	}
}

func TestResolveToken_TerraformConfigCredentials(t *testing.T) {
	env := &mockEnv{vars: map[string]string{}}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/home/testuser/.terraformrc": `
credentials "app.terraform.io" {
  token = "config-token"
}
`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "config-token" {
		t.Errorf("Token = %q, want %q", result.Token, "config-token")
	}
	if result.Source != SourceTerraformConfig {
		t.Errorf("Source = %q, want %q", result.Source, SourceTerraformConfig)
	}
}

func TestResolveToken_TF_CLI_CONFIG_FILE(t *testing.T) {
	env := &mockEnv{
		vars: map[string]string{
			"TF_CLI_CONFIG_FILE": "/custom/terraform.rc",
		},
	}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/custom/terraform.rc": `
credentials "app.terraform.io" {
  token = "custom-config-token"
}
`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "custom-config-token" {
		t.Errorf("Token = %q, want %q", result.Token, "custom-config-token")
	}
	if result.Source != SourceTerraformConfig {
		t.Errorf("Source = %q, want %q", result.Source, SourceTerraformConfig)
	}
}

func TestResolveToken_AddressWithPath(t *testing.T) {
	// Address includes path (e.g., app.terraform.io/eu) but token lookup uses hostname only
	env := &mockEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "eu-token",
		},
	}
	fs := &mockFS{}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io/eu")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "eu-token" {
		t.Errorf("Token = %q, want %q", result.Token, "eu-token")
	}
}

func TestResolveToken_FullURL(t *testing.T) {
	env := &mockEnv{
		vars: map[string]string{
			"TF_TOKEN_tfe_example_com": "tfe-token",
		},
	}
	fs := &mockFS{}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("https://tfe.example.com")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "tfe-token" {
		t.Errorf("Token = %q, want %q", result.Token, "tfe-token")
	}
}

func TestResolveToken_Precedence_EnvOverConfig(t *testing.T) {
	env := &mockEnv{
		vars: map[string]string{
			"TF_TOKEN_app_terraform_io": "env-token",
		},
	}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/home/testuser/.terraformrc": `
credentials "app.terraform.io" {
  token = "config-token"
}
`,
			"/home/testuser/.terraform.d/credentials.tfrc.json": `{
				"credentials": {
					"app.terraform.io": {
						"token": "file-token"
					}
				}
			}`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "env-token" {
		t.Errorf("Token = %q, want %q (env should win)", result.Token, "env-token")
	}
	if result.Source != SourceEnv {
		t.Errorf("Source = %q, want %q", result.Source, SourceEnv)
	}
}

func TestResolveToken_Precedence_ConfigOverCredentialsFile(t *testing.T) {
	env := &mockEnv{vars: map[string]string{}}
	fs := &mockFS{
		homeDir: "/home/testuser",
		files: map[string]string{
			"/home/testuser/.terraformrc": `
credentials "app.terraform.io" {
  token = "config-token"
}
`,
			"/home/testuser/.terraform.d/credentials.tfrc.json": `{
				"credentials": {
					"app.terraform.io": {
						"token": "file-token"
					}
				}
			}`,
		},
	}

	resolver := &TokenResolver{Env: env, FS: fs}
	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "config-token" {
		t.Errorf("Token = %q, want %q (config should win over credentials file)", result.Token, "config-token")
	}
	if result.Source != SourceTerraformConfig {
		t.Errorf("Source = %q, want %q", result.Source, SourceTerraformConfig)
	}
}

func TestResolveToken_InvalidAddress(t *testing.T) {
	resolver := NewTokenResolver()
	_, err := resolver.ResolveToken("")

	if err == nil {
		t.Fatal("ResolveToken() error = nil, want error for empty address")
	}
}
