package tfcapi

import (
	"errors"
	"testing"

	tfe "github.com/hashicorp/go-tfe"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		apiErr   *APIError
		expected string
	}{
		{
			name: "with title and detail",
			apiErr: &APIError{
				Status: 401,
				Title:  "Unauthorized",
				Detail: "Invalid API token",
			},
			expected: "Unauthorized: Invalid API token",
		},
		{
			name: "with title only",
			apiErr: &APIError{
				Status: 404,
				Title:  "Not Found",
			},
			expected: "Not Found",
		},
		{
			name: "with errors array",
			apiErr: &APIError{
				Status: 422,
				Errors: []APIErrorItem{
					{Detail: "Name is required"},
					{Detail: "Email is invalid"},
				},
			},
			expected: "Name is required; Email is invalid",
		},
		{
			name: "with status only",
			apiErr: &APIError{
				Status: 500,
			},
			expected: "API error (status 500)",
		},
		{
			name: "errors array with titles only",
			apiErr: &APIError{
				Status: 400,
				Errors: []APIErrorItem{
					{Title: "Validation Error"},
				},
			},
			expected: "Validation Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.apiErr.Error()
			if got != tt.expected {
				t.Errorf("APIError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseAPIError_KnownErrors(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedTitle  string
	}{
		{
			name:           "unauthorized error",
			err:            tfe.ErrUnauthorized,
			expectedStatus: 401,
			expectedTitle:  "Unauthorized",
		},
		{
			name:           "resource not found error",
			err:            tfe.ErrResourceNotFound,
			expectedStatus: 404,
			expectedTitle:  "Not Found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiErr, parseErr := ParseAPIError(tt.err)
			if parseErr != nil {
				t.Errorf("ParseAPIError() returned parse error: %v", parseErr)
				return
			}
			if apiErr == nil {
				t.Error("ParseAPIError() returned nil APIError")
				return
			}
			if apiErr.Status != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.expectedStatus)
			}
			if apiErr.Title != tt.expectedTitle {
				t.Errorf("Title = %q, want %q", apiErr.Title, tt.expectedTitle)
			}
		})
	}
}

func TestParseAPIError_NilError(t *testing.T) {
	apiErr, parseErr := ParseAPIError(nil)
	if apiErr != nil {
		t.Error("ParseAPIError(nil) should return nil APIError")
	}
	if parseErr != nil {
		t.Error("ParseAPIError(nil) should return nil parse error")
	}
}

func TestParseAPIError_GenericError(t *testing.T) {
	tests := []struct {
		name           string
		errMsg         string
		expectedStatus int
		expectedTitle  string
	}{
		{
			name:           "unauthorized in message",
			errMsg:         "request failed: unauthorized access",
			expectedStatus: 401,
			expectedTitle:  "Unauthorized",
		},
		{
			name:           "not found in message",
			errMsg:         "resource not found",
			expectedStatus: 404,
			expectedTitle:  "Not Found",
		},
		{
			name:           "rate limit in message",
			errMsg:         "rate limit exceeded",
			expectedStatus: 429,
			expectedTitle:  "Rate Limited",
		},
		{
			name:           "service unavailable in message",
			errMsg:         "service unavailable, try again later",
			expectedStatus: 503,
			expectedTitle:  "Service Unavailable",
		},
		{
			name:           "forbidden in message",
			errMsg:         "forbidden: insufficient permissions",
			expectedStatus: 403,
			expectedTitle:  "Forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			apiErr, parseErr := ParseAPIError(err)
			if parseErr != nil {
				t.Errorf("ParseAPIError() returned parse error: %v", parseErr)
				return
			}
			if apiErr == nil {
				t.Error("ParseAPIError() returned nil APIError")
				return
			}
			if apiErr.Status != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", apiErr.Status, tt.expectedStatus)
			}
			if apiErr.Title != tt.expectedTitle {
				t.Errorf("Title = %q, want %q", apiErr.Title, tt.expectedTitle)
			}
		})
	}
}

func TestParseAPIError_JSONAPIInMessage(t *testing.T) {
	// Test parsing JSON:API error body embedded in error message
	errMsg := `request failed: {"errors":[{"status":"401","title":"Unauthorized","detail":"Invalid API token provided"}]}`
	err := errors.New(errMsg)

	apiErr, parseErr := ParseAPIError(err)
	if parseErr != nil {
		t.Errorf("ParseAPIError() returned parse error: %v", parseErr)
		return
	}
	if apiErr == nil {
		t.Error("ParseAPIError() returned nil APIError")
		return
	}
	if apiErr.Status != 401 {
		t.Errorf("Status = %d, want 401", apiErr.Status)
	}
	if apiErr.Detail != "Invalid API token provided" {
		t.Errorf("Detail = %q, want %q", apiErr.Detail, "Invalid API token provided")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limited",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "service unavailable",
			err:      errors.New("service unavailable"),
			expected: true,
		},
		{
			name:     "unauthorized is not retryable",
			err:      tfe.ErrUnauthorized,
			expected: false,
		},
		{
			name:     "not found is not retryable",
			err:      tfe.ErrResourceNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDetectErrorType(t *testing.T) {
	tests := []struct {
		detail         string
		expectedStatus int
		expectedTitle  string
	}{
		{"unauthorized", 401, "Unauthorized"},
		{"authentication failed", 401, "Unauthorized"},
		{"forbidden", 403, "Forbidden"},
		{"permission denied", 403, "Forbidden"},
		{"not found", 404, "Not Found"},
		{"rate limit", 429, "Rate Limited"},
		{"too many requests", 429, "Rate Limited"},
		{"service unavailable", 503, "Service Unavailable"},
		{"internal server error", 500, "Internal Server Error"},
		{"unknown error", 0, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.detail, func(t *testing.T) {
			status, title := detectErrorType(tt.detail)
			if status != tt.expectedStatus {
				t.Errorf("detectErrorType(%q) status = %d, want %d", tt.detail, status, tt.expectedStatus)
			}
			if title != tt.expectedTitle {
				t.Errorf("detectErrorType(%q) title = %q, want %q", tt.detail, title, tt.expectedTitle)
			}
		})
	}
}
