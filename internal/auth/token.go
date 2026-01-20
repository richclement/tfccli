package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TokenSource describes where a token was discovered.
type TokenSource string

const (
	SourceEnv             TokenSource = "env"
	SourceTerraformConfig TokenSource = "terraform config"
	SourceCredentialsFile TokenSource = "credentials.tfrc.json"
)

// TokenResult holds the discovered token and its source.
type TokenResult struct {
	Token  string
	Source TokenSource
}

// EnvGetter abstracts environment variable access for testing.
type EnvGetter interface {
	Getenv(key string) string
}

// FSReader abstracts filesystem reading for testing.
type FSReader interface {
	ReadFile(path string) ([]byte, error)
	UserHomeDir() (string, error)
}

// DefaultEnv implements EnvGetter using os.Getenv.
type DefaultEnv struct{}

func (DefaultEnv) Getenv(key string) string { return os.Getenv(key) }

// DefaultFS implements FSReader using os functions.
type DefaultFS struct{}

func (DefaultFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (DefaultFS) UserHomeDir() (string, error)         { return os.UserHomeDir() }

// TokenResolver discovers Terraform API tokens.
type TokenResolver struct {
	Env EnvGetter
	FS  FSReader
}

// NewTokenResolver creates a TokenResolver with default env and fs.
func NewTokenResolver() *TokenResolver {
	return &TokenResolver{
		Env: DefaultEnv{},
		FS:  DefaultFS{},
	}
}

// ResolveToken discovers a Terraform API token for the given address.
// It follows Terraform CLI conventions with this precedence:
// 1. Environment variable TF_TOKEN_<sanitized_host>
// 2. Terraform CLI config credentials block (TF_CLI_CONFIG_FILE or default location)
// 3. credentials.tfrc.json (produced by terraform login)
//
// Returns an actionable error if no token is found.
func (r *TokenResolver) ResolveToken(address string) (*TokenResult, error) {
	hostname, err := ExtractHostname(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", address, err)
	}

	// 1. Check environment variable
	if token := r.checkEnvToken(hostname); token != "" {
		return &TokenResult{Token: token, Source: SourceEnv}, nil
	}

	// 2. Check Terraform CLI config
	if token := r.checkTerraformConfig(hostname); token != "" {
		return &TokenResult{Token: token, Source: SourceTerraformConfig}, nil
	}

	// 3. Check credentials.tfrc.json
	if token := r.checkCredentialsFile(hostname); token != "" {
		return &TokenResult{Token: token, Source: SourceCredentialsFile}, nil
	}

	return nil, &NoTokenError{Hostname: hostname}
}

// NoTokenError is returned when no token can be discovered.
type NoTokenError struct {
	Hostname string
}

func (e *NoTokenError) Error() string {
	return fmt.Sprintf(
		"no API token found for %q. Run 'terraform login %s' to authenticate",
		e.Hostname, e.Hostname,
	)
}

// ExtractHostname parses an address and returns just the hostname.
// Accepts formats like:
//   - app.terraform.io
//   - https://app.terraform.io
//   - app.terraform.io/eu
//   - https://tfe.example.com:8443/path
func ExtractHostname(address string) (string, error) {
	if address == "" {
		return "", errors.New("address is empty")
	}

	// If no scheme, add one for parsing
	parseAddr := address
	if !strings.Contains(address, "://") {
		parseAddr = "https://" + address
	}

	u, err := url.Parse(parseAddr)
	if err != nil {
		return "", err
	}

	hostname := u.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("could not extract hostname from %q", address)
	}

	return hostname, nil
}

// SanitizeHost converts a hostname to the format used in TF_TOKEN_ env vars.
// Replaces '.' and '-' with '_'.
func SanitizeHost(hostname string) string {
	s := strings.ReplaceAll(hostname, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func (r *TokenResolver) checkEnvToken(hostname string) string {
	envKey := "TF_TOKEN_" + SanitizeHost(hostname)
	return r.Env.Getenv(envKey)
}

func (r *TokenResolver) checkTerraformConfig(hostname string) string {
	// Check TF_CLI_CONFIG_FILE first
	configPath := r.Env.Getenv("TF_CLI_CONFIG_FILE")
	if configPath != "" {
		if token := r.parseHCLCredentials(configPath, hostname); token != "" {
			return token
		}
	}

	// Check default terraform config locations
	home, err := r.FS.UserHomeDir()
	if err != nil {
		return ""
	}

	var paths []string
	if runtime.GOOS == "windows" {
		// Windows: %APPDATA%\terraform.rc
		appdata := r.Env.Getenv("APPDATA")
		if appdata != "" {
			paths = append(paths, filepath.Join(appdata, "terraform.rc"))
		}
	} else {
		// Unix: ~/.terraformrc
		paths = append(paths, filepath.Join(home, ".terraformrc"))
	}

	for _, path := range paths {
		if token := r.parseHCLCredentials(path, hostname); token != "" {
			return token
		}
	}

	return ""
}

// parseHCLCredentials parses Terraform CLI config for credentials.
// The format is:
//
//	credentials "app.terraform.io" {
//	  token = "xxxxxx.atlasv1.zzzzzzzzzzzzz"
//	}
//
// For simplicity, we use a basic parser rather than pulling in HCL deps.
func (r *TokenResolver) parseHCLCredentials(path, hostname string) string {
	data, err := r.FS.ReadFile(path)
	if err != nil {
		return ""
	}

	content := string(data)

	// Find credentials block for hostname
	// Pattern: credentials "hostname" { ... token = "..." ... }
	target := fmt.Sprintf(`credentials "%s"`, hostname)
	_, rest, found := strings.Cut(content, target)
	if !found {
		return ""
	}

	// Find opening brace
	_, rest, found = strings.Cut(rest, "{")
	if !found {
		return ""
	}

	// Find closing brace
	block, _, found := strings.Cut(rest, "}")
	if !found {
		return ""
	}

	// Find token = "..."
	_, tokenRest, found := strings.Cut(block, "token")
	if !found {
		return ""
	}

	// Find the value after =
	_, tokenRest, found = strings.Cut(tokenRest, "=")
	if !found {
		return ""
	}

	tokenRest = strings.TrimSpace(tokenRest)

	// Extract quoted string
	if len(tokenRest) < 2 || tokenRest[0] != '"' {
		return ""
	}

	endQuote := strings.Index(tokenRest[1:], `"`)
	if endQuote == -1 {
		return ""
	}

	return tokenRest[1 : endQuote+1]
}

func (r *TokenResolver) checkCredentialsFile(hostname string) string {
	home, err := r.FS.UserHomeDir()
	if err != nil {
		return ""
	}

	// credentials.tfrc.json is at ~/.terraform.d/credentials.tfrc.json
	credPath := filepath.Join(home, ".terraform.d", "credentials.tfrc.json")
	data, err := r.FS.ReadFile(credPath)
	if err != nil {
		return ""
	}

	// Parse JSON structure:
	// {
	//   "credentials": {
	//     "app.terraform.io": {
	//       "token": "xxxxxx.atlasv1.zzzzzzzzzzzzz"
	//     }
	//   }
	// }
	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return ""
	}

	if hostCreds, ok := creds.Credentials[hostname]; ok {
		return hostCreds.Token
	}

	return ""
}

type credentialsFile struct {
	Credentials map[string]hostCredentials `json:"credentials"`
}

type hostCredentials struct {
	Token string `json:"token"`
}
