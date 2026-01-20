package tfcapi

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	tfe "github.com/hashicorp/go-tfe"
)

// PingableClient is an interface for clients that can be pinged.
type PingableClient interface {
	Ping(ctx context.Context) error
}

// ClientConfig holds the configuration for creating a TFC client.
type ClientConfig struct {
	// Address is the Terraform Cloud/Enterprise address (e.g., "app.terraform.io")
	Address string
	// Token is the API token for authentication
	Token string
	// Logger is the logger for request logging
	Logger logr.Logger
}

// NewClient creates a new Terraform Cloud/Enterprise API client.
// The address is normalized (https:// added if missing) before use.
// The token is used for authentication with the API.
func NewClient(cfg ClientConfig) (*tfe.Client, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("token is required")
	}

	address := NormalizeAddress(cfg.Address)
	if address == "" {
		address = "https://app.terraform.io"
	}

	config := &tfe.Config{
		Address:           address,
		Token:             cfg.Token,
		RetryServerErrors: true, // Enable retry on 5xx errors
	}

	client, err := tfe.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TFC client: %w", err)
	}

	return client, nil
}

// Client wraps the go-tfe client with additional functionality.
type Client struct {
	*tfe.Client
}

// NewClientWithWrapper creates a wrapped TFC client.
func NewClientWithWrapper(cfg ClientConfig) (*Client, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{Client: client}, nil
}

// Ping verifies connectivity to the TFC API.
// Returns nil if the connection is successful.
func (c *Client) Ping(ctx context.Context) error {
	// Use organizations list as a lightweight connectivity check
	_, err := c.Organizations.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("TFC API connectivity check failed: %w", err)
	}
	return nil
}

// Ping verifies connectivity to the TFC API using a raw go-tfe client.
// Returns nil if the connection is successful.
func Ping(ctx context.Context, client *tfe.Client) error {
	// Use organizations list as a lightweight connectivity check
	_, err := client.Organizations.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("TFC API connectivity check failed: %w", err)
	}
	return nil
}
