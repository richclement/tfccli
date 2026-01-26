package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/richclement/tfccli/internal/auth"
	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/output"
	"github.com/richclement/tfccli/internal/tfcapi"
)

// UsersCmd groups all users subcommands.
type UsersCmd struct {
	Get UsersGetCmd `cmd:"" help:"Get a user by ID."`
	Me  UsersMeCmd  `cmd:"" help:"Get the current authenticated user."`
}

// usersClient abstracts the TFC users API for testing.
type usersClient interface {
	Read(ctx context.Context, userID string) (*UserResponse, error)
	ReadCurrentUser(ctx context.Context) (*UserResponse, error)
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

// realUsersClient implements usersClient using the shared HTTP client.
// go-tfe doesn't expose GET /users/:id, so we make the call directly.
type realUsersClient struct {
	httpClient *tfcapi.HTTPClient
}

func (c *realUsersClient) Read(ctx context.Context, userID string) (*UserResponse, error) {
	path := fmt.Sprintf("/api/v2/users/%s", url.PathEscape(userID))

	body, err := c.httpClient.DoRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var userResp UserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &userResp, nil
}

func (c *realUsersClient) ReadCurrentUser(ctx context.Context) (*UserResponse, error) {
	body, err := c.httpClient.DoRequest(ctx, "GET", "/api/v2/account/details", nil)
	if err != nil {
		return nil, err
	}

	var userResp UserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &userResp, nil
}

// defaultUsersClientFactory creates a real TFC client that satisfies usersClient.
func defaultUsersClientFactory(cfg tfcapi.ClientConfig) (usersClient, error) {
	return &realUsersClient{
		httpClient: tfcapi.NewHTTPClient(cfg),
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

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	user, err := client.Read(ctx, c.UserID)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get user: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get user: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

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

// UsersMeCmd gets the current authenticated user.
type UsersMeCmd struct {
	// Dependencies for testing
	baseDir       string
	tokenResolver *auth.TokenResolver
	ttyDetector   output.TTYDetector
	stdout        io.Writer
	clientFactory usersClientFactory
}

func (c *UsersMeCmd) Run(cli *CLI) error {
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

	cfg, _, err := resolveClientConfig(cli, c.baseDir, c.tokenResolver)
	if err != nil {
		return internalcmd.NewRuntimeError(err)
	}

	client, err := c.clientFactory(cfg)
	if err != nil {
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to create client: %w", err))
	}

	ctx := cmdContext(cli)
	user, err := client.ReadCurrentUser(ctx)
	if err != nil {
		apiErr, _ := tfcapi.ParseAPIError(err)
		if apiErr != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to get current user: %w", apiErr))
		}
		return internalcmd.NewRuntimeError(fmt.Errorf("failed to get current user: %w", err))
	}

	format, isTTY := resolveFormat(c.stdout, c.ttyDetector, cli.OutputFormat)

	if format == output.FormatJSON {
		if err := output.WriteJSON(c.stdout, user); err != nil {
			return internalcmd.NewRuntimeError(fmt.Errorf("failed to write output: %w", err))
		}
	} else {
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
