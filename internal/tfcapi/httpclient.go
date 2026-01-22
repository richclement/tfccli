package tfcapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPClient is a shared HTTP client for making TFC API requests.
// It handles request construction with auth headers and common error parsing.
type HTTPClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewHTTPClient creates a new HTTPClient for TFC API requests.
func NewHTTPClient(cfg ClientConfig) *HTTPClient {
	baseURL := NormalizeAddress(cfg.Address)
	if baseURL == "" {
		baseURL = "https://app.terraform.io"
	}

	return &HTTPClient{
		BaseURL:    baseURL,
		Token:      cfg.Token,
		HTTPClient: &http.Client{},
	}
}

// DoRequest performs an HTTP request with authentication and returns the response body.
// It handles common error status codes and JSON:API error parsing.
func (c *HTTPClient) DoRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	url := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if err := ParseHTTPError(resp.StatusCode, respBody); err != nil {
		return nil, err
	}

	return respBody, nil
}

// JSONAPIError represents a single error from a JSON:API error response.
type JSONAPIError struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// JSONAPIErrorResponse represents a JSON:API error response body.
type JSONAPIErrorResponse struct {
	Errors []JSONAPIError `json:"errors"`
}

// ParseHTTPError parses an HTTP response status code and body into an error.
// Returns nil if the status code indicates success (2xx).
func ParseHTTPError(statusCode int, body []byte) error {
	// Success codes: 2xx
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}

	switch statusCode {
	case http.StatusNotFound:
		return fmt.Errorf("resource not found")
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized: invalid or missing API token")
	case http.StatusForbidden:
		return fmt.Errorf("forbidden: insufficient permissions")
	}

	// Try to parse JSON:API error
	var errResp JSONAPIErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
		e := errResp.Errors[0]
		if e.Detail != "" {
			return fmt.Errorf("%s: %s", e.Title, e.Detail)
		}
		return fmt.Errorf("%s", e.Title)
	}

	return fmt.Errorf("API request failed with status %d", statusCode)
}

// ParseJSONAPIErrorResponse attempts to parse a JSON:API error from a response body.
// Returns nil if parsing fails or no errors are present.
func ParseJSONAPIErrorResponse(body []byte) *JSONAPIErrorResponse {
	var errResp JSONAPIErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && len(errResp.Errors) > 0 {
		return &errResp
	}
	return nil
}
