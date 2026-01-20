package testutil

import (
	"os"

	"github.com/richclement/tfccli/internal/auth"
)

// Verify interface compliance at compile time.
var (
	_ auth.EnvGetter = (*FakeEnv)(nil)
	_ auth.FSReader  = (*FakeFS)(nil)
)

// FakeEnv implements auth.EnvGetter for tests with pre-configured values.
type FakeEnv struct {
	Vars map[string]string
}

// NewFakeEnv creates a FakeEnv with the given environment variables.
func NewFakeEnv(vars map[string]string) *FakeEnv {
	if vars == nil {
		vars = make(map[string]string)
	}
	return &FakeEnv{Vars: vars}
}

// Getenv returns the value for the given key, or empty string if not set.
func (e *FakeEnv) Getenv(key string) string {
	return e.Vars[key]
}

// Set adds or updates an environment variable.
func (e *FakeEnv) Set(key, value string) {
	e.Vars[key] = value
}

// FakeFS implements auth.FSReader for tests with pre-configured files.
type FakeFS struct {
	Files   map[string][]byte
	HomeDir string
}

// NewFakeFS creates a FakeFS with the given home directory and optional files.
func NewFakeFS(homeDir string, files map[string][]byte) *FakeFS {
	if files == nil {
		files = make(map[string][]byte)
	}
	return &FakeFS{
		HomeDir: homeDir,
		Files:   files,
	}
}

// ReadFile returns the content of the given path, or os.ErrNotExist if not found.
func (f *FakeFS) ReadFile(path string) ([]byte, error) {
	if data, ok := f.Files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

// UserHomeDir returns the configured home directory.
func (f *FakeFS) UserHomeDir() (string, error) {
	return f.HomeDir, nil
}

// AddFile adds a file with the given content.
func (f *FakeFS) AddFile(path string, content []byte) {
	f.Files[path] = content
}

// AddFileString adds a file with string content.
func (f *FakeFS) AddFileString(path, content string) {
	f.Files[path] = []byte(content)
}

// NewTestTokenResolver creates a TokenResolver with the given env vars and files.
// This is a convenience function for common test setups.
func NewTestTokenResolver(homeDir string, envVars map[string]string, files map[string][]byte) *auth.TokenResolver {
	return &auth.TokenResolver{
		Env: NewFakeEnv(envVars),
		FS:  NewFakeFS(homeDir, files),
	}
}

// TokenResolverWithEnvToken creates a TokenResolver that returns the given
// token from the environment for the given host.
// Example: TokenResolverWithEnvToken(tmpDir, "app.terraform.io", "test-token")
func TokenResolverWithEnvToken(homeDir, host, token string) *auth.TokenResolver {
	sanitizedHost := sanitizeHost(host)
	envKey := "TF_TOKEN_" + sanitizedHost

	return NewTestTokenResolver(homeDir, map[string]string{envKey: token}, nil)
}

// sanitizeHost converts a hostname to the Terraform env var format.
// Example: "app.terraform.io" -> "app_terraform_io"
func sanitizeHost(host string) string {
	result := make([]byte, len(host))
	for i := range len(host) {
		c := host[i]
		if c == '.' || c == '-' {
			result[i] = '_'
		} else {
			result[i] = c
		}
	}
	return string(result)
}
