package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// UsersCmd groups all users subcommands.
type UsersCmd struct {
	Get UsersGetCmd `cmd:"" help:"Get a user by ID."`
}

// usersClient abstracts the TFC users API for testing.
type usersClient interface {
	Read(ctx context.Context, userID string) (*UserResponse, error)
}

// UserResponse represents the JSON:API response for a user.
type UserResponse struct {
	Data UserData `json:"data"`
}

// UserData represents a user in JSON:API format.
type UserData struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Attributes UserAttributes `json:"attributes"`
}

// UserAttributes represents user attributes.
type UserAttributes struct {
	Username         string `json:"username"`
	Email            string `json:"email,omitempty"`
	AvatarURL        string `json:"avatar-url,omitempty"`
	IsServiceAccount bool   `json:"is-service-account"`
	V2Only           bool   `json:"v2-only"`
}

// usersClientFactory creates a usersClient from config.
type usersClientFactory func(cfg tfcapi.ClientConfig) (usersClient, error)

// realUsersClient implements usersClient using raw HTTP calls.
// go-tfe doesn't expose GET /users/:id, so we make the call directly.
type realUsersClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func (c *realUsersClient) Read(ctx context.Context, userID string) (*UserResponse, error) {
	apiURL := fmt.Sprintf("%s/api/v2/users/%s", c.baseURL, url.PathEscape(userID))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: invalid or missing API token")
	}
	if resp.StatusCode != http.StatusOK {
		// Try to parse JSON:API error
		var errResp struct {
			Errors []struct {
				Status string `json:"status"`
				Title  string `json:"title"`
				Detail string `json:"detail"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
			return nil, fmt.Errorf("%s: %s", errResp.Errors[0].Title, errResp.Errors[0].Detail)
		}
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var userResp UserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &userResp, nil
}

// defaultUsersClientFactory creates a real TFC client that satisfies usersClient.
func defaultUsersClientFactory(cfg tfcapi.ClientConfig) (usersClient, error) {
	baseURL := tfcapi.NormalizeAddress(cfg.Address)
	if baseURL == "" {
		baseURL = "https://app.terraform.io"
	}

	return &realUsersClient{
		baseURL:    baseURL,
		token:      cfg.Token,
		httpClient: &http.Client{},
	}, nil
}

// resolveUsersClientConfig resolves settings and token for API calls.
func resolveUsersClientConfig(cli *CLI, baseDir string, tokenResolver *auth.TokenResolver) (tfcapi.ClientConfig, error) {
	settings, err := config.Load(baseDir)
	if err != nil {
		return tfcapi.ClientConfig{}, err
	}

	contextName := cli.Context
	if contextName == "" {
		contextName = settings.CurrentContext
	}
	ctx, exists := settings.Contexts[contextName]
	if !exists {
		return tfcapi.ClientConfig{}, fmt.Errorf("context %q not found", contextName)
	}

	resolved := ctx.WithDefaults()
	if cli.Address != "" {
		resolved.Address = cli.Address
	}

	if tokenResolver == nil {
		tokenResolver = auth.NewTokenResolver()
	}
	tokenResult, err := tokenResolver.ResolveToken(resolved.Address)
	if err != nil {
		return tfcapi.ClientConfig{}, err
	}

	return tfcapi.ClientConfig{
		Address: resolved.Address,
		Token:   tokenResult.Token,
	}, nil
}

// UsersGetCmd gets a user by ID.
type UsersGetCmd struct {
	UserID string `arg:"" help:"ID of the user."`

	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory usersClientFactory
}

func (c *UsersGetCmd) Run(cli *CLI) error {
	// Set defaults
	if c.ttyDetector == nil {
		c.ttyDetector = &output.RealTTYDetector{}
	}
	if c.stdout == nil {
		c.stdout = os.Stdout
	}
	if c.clientFactory == nil {
		c.clientFactory = defaultUsersClientFactory
	}

	cfg, err := resolveUsersClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := context.Background()
	user, err := client.Read(ctx, c.UserID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get user: %s", apiErr.Error()))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get user: %w", err))
	}

	// Determine output format
	isTTY := false
	if f, ok := c.stdout.(*os.File); ok {
		isTTY = c.ttyDetector.IsTTY(f)
	}
	format := output.ResolveOutputFormat(cli.OutputFormat, isTTY)

	if format == output.FormatJSON {
		// Emit raw JSON:API response
		if err := output.WriteJSON(c.stdout, user); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
		// Table output: show key fields
		tw := output.NewTableWriter(c.stdout, []string{"FIELD", "VALUE"}, isTTY)
		tw.AddRow("ID", user.Data.ID)
		tw.AddRow("Username", user.Data.Attributes.Username)
		tw.AddRow("Email", user.Data.Attributes.Email)
		tw.AddRow("Avatar URL", user.Data.Attributes.AvatarURL)
		tw.AddRow("Service Account", fmt.Sprintf("%t", user.Data.Attributes.IsServiceAccount))
		if _, err := tw.Render(); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	}

	return nil
}
