package tfcapi

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	tfe "github.com/hashicorp/go-tfe"
)

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

// Ping verifies connectivity to the TFC API.
// Returns nil if the connection is successful.
func Ping(ctx context.Context, client *tfe.Client) error {
	// Use organizations list as a lightweight connectivity check
	_, err := client.Organizations.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("TFC API connectivity check failed: %w", err)
	}
	return nil
}
