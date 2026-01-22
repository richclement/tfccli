package tfcapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name            string
		cfg             ClientConfig
		expectedBaseURL string
	}{
		{
			name: "with address",
			cfg: ClientConfig{
				Address: "https://tfe.example.com",
				Token:   "test-token",
			},
			expectedBaseURL: "https://tfe.example.com",
		},
		{
			name: "empty address uses default",
			cfg: ClientConfig{
				Address: "",
				Token:   "test-token",
			},
			expectedBaseURL: "https://app.terraform.io",
		},
		{
			name: "address without scheme",
			cfg: ClientConfig{
				Address: "tfe.example.com",
				Token:   "test-token",
			},
			expectedBaseURL: "https://tfe.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewHTTPClient(tt.cfg)
			if client.BaseURL != tt.expectedBaseURL {
				t.Errorf("BaseURL = %q, want %q", client.BaseURL, tt.expectedBaseURL)
			}
			if client.Token != tt.cfg.Token {
				t.Errorf("Token = %q, want %q", client.Token, tt.cfg.Token)
			}
			if client.HTTPClient == nil {
				t.Error("HTTPClient should not be nil")
			}
		})
	}
}

func TestHTTPClient_DoRequest(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectError    bool
		errorContains  string
		expectedResult string
	}{
		{
			name:           "success",
			statusCode:     http.StatusOK,
			responseBody:   `{"data":"test"}`,
			expectError:    false,
			expectedResult: `{"data":"test"}`,
		},
		{
			name:           "201 created success",
			statusCode:     http.StatusCreated,
			responseBody:   `{"data":"created"}`,
			expectError:    false,
			expectedResult: `{"data":"created"}`,
		},
		{
			name:          "unauthorized",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{}`,
			expectError:   true,
			errorContains: "unauthorized",
		},
		{
			name:          "not found",
			statusCode:    http.StatusNotFound,
			responseBody:  `{}`,
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "forbidden",
			statusCode:    http.StatusForbidden,
			responseBody:  `{}`,
			expectError:   true,
			errorContains: "forbidden",
		},
		{
			name:         "json api error",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"errors":[{"status":"400","title":"Bad Request","detail":"Invalid parameter"}]}`,
			expectError:  true,
			// Should contain the parsed JSON:API error
			errorContains: "Invalid parameter",
		},
		{
			name:          "generic error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `not json`,
			expectError:   true,
			errorContains: "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify auth header
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Authorization header = %q, want %q", r.Header.Get("Authorization"), "Bearer test-token")
				}
				// Verify content type
				if r.Header.Get("Content-Type") != "application/vnd.api+json" {
					t.Errorf("Content-Type header = %q, want %q", r.Header.Get("Content-Type"), "application/vnd.api+json")
				}

				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := &HTTPClient{
				BaseURL:    server.URL,
				Token:      "test-token",
				HTTPClient: &http.Client{},
			}

			result, err := client.DoRequest(context.Background(), "GET", "/test", nil)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorContains != "" && !containsCI(err.Error(), tt.errorContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(result) != tt.expectedResult {
					t.Errorf("result = %q, want %q", string(result), tt.expectedResult)
				}
			}
		})
	}
}

func TestParseHTTPError(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		body          []byte
		expectError   bool
		errorContains string
	}{
		{
			name:        "200 OK",
			statusCode:  http.StatusOK,
			body:        []byte(`{}`),
			expectError: false,
		},
		{
			name:        "201 Created",
			statusCode:  http.StatusCreated,
			body:        []byte(`{}`),
			expectError: false,
		},
		{
			name:        "204 No Content",
			statusCode:  http.StatusNoContent,
			body:        nil,
			expectError: false,
		},
		{
			name:          "401 Unauthorized",
			statusCode:    http.StatusUnauthorized,
			body:          []byte(`{}`),
			expectError:   true,
			errorContains: "unauthorized",
		},
		{
			name:          "403 Forbidden",
			statusCode:    http.StatusForbidden,
			body:          []byte(`{}`),
			expectError:   true,
			errorContains: "forbidden",
		},
		{
			name:          "404 Not Found",
			statusCode:    http.StatusNotFound,
			body:          []byte(`{}`),
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "JSON:API error with detail",
			statusCode:    http.StatusBadRequest,
			body:          []byte(`{"errors":[{"status":"400","title":"Bad Request","detail":"Missing required field"}]}`),
			expectError:   true,
			errorContains: "Missing required field",
		},
		{
			name:          "JSON:API error title only",
			statusCode:    http.StatusBadRequest,
			body:          []byte(`{"errors":[{"status":"400","title":"Validation Failed"}]}`),
			expectError:   true,
			errorContains: "Validation Failed",
		},
		{
			name:          "generic 500 error",
			statusCode:    http.StatusInternalServerError,
			body:          []byte(`not json`),
			expectError:   true,
			errorContains: "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseHTTPError(tt.statusCode, tt.body)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorContains != "" && !containsCI(err.Error(), tt.errorContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errorContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestParseJSONAPIErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		body       []byte
		wantNil    bool
		wantErrors int
	}{
		{
			name:       "valid error response",
			body:       []byte(`{"errors":[{"status":"400","title":"Bad Request","detail":"Invalid"}]}`),
			wantNil:    false,
			wantErrors: 1,
		},
		{
			name:       "multiple errors",
			body:       []byte(`{"errors":[{"title":"Error 1"},{"title":"Error 2"}]}`),
			wantNil:    false,
			wantErrors: 2,
		},
		{
			name:    "empty errors array",
			body:    []byte(`{"errors":[]}`),
			wantNil: true,
		},
		{
			name:    "invalid json",
			body:    []byte(`not json`),
			wantNil: true,
		},
		{
			name:    "no errors field",
			body:    []byte(`{"data":"test"}`),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseJSONAPIErrorResponse(tt.body)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
			} else {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if len(result.Errors) != tt.wantErrors {
					t.Errorf("got %d errors, want %d", len(result.Errors), tt.wantErrors)
				}
			}
		})
	}
}

// containsCI does a case-insensitive substring check.
func containsCI(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(substr) == 0 ||
			(len(s) > 0 && containsCIHelper(s, substr)))
}

func containsCIHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFoldSlice(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFoldSlice(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
