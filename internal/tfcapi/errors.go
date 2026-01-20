package tfcapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	tfe "github.com/hashicorp/go-tfe"
)

// APIError represents a structured error from the TFC/TFE JSON:API.
type APIError struct {
	// Status is the HTTP status code (e.g., 401, 404, 500)
	Status int `json:"status"`
	// Title is the short error title (e.g., "Unauthorized")
	Title string `json:"title"`
	// Detail provides additional error context
	Detail string `json:"detail"`
	// Errors contains individual JSON:API error objects
	Errors []APIErrorItem `json:"errors,omitempty"`
}

// APIErrorItem represents a single error in the JSON:API errors array.
type APIErrorItem struct {
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Source *struct {
		Pointer   string `json:"pointer,omitempty"`
		Parameter string `json:"parameter,omitempty"`
	} `json:"source,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Title, e.Detail)
	}
	if len(e.Errors) > 0 {
		var parts []string
		for _, err := range e.Errors {
			if err.Detail != "" {
				parts = append(parts, err.Detail)
			} else if err.Title != "" {
				parts = append(parts, err.Title)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "; ")
		}
	}
	if e.Title != "" {
		return e.Title
	}
	return fmt.Sprintf("API error (status %d)", e.Status)
}

// jsonAPIErrorResponse is the structure of a JSON:API error response body.
type jsonAPIErrorResponse struct {
	Errors []APIErrorItem `json:"errors"`
}

// ParseAPIError attempts to extract a structured APIError from a go-tfe error.
// If the error cannot be parsed as an API error, returns nil and the original error.
func ParseAPIError(err error) (*APIError, error) {
	if err == nil {
		return nil, nil
	}

	// Check for known go-tfe errors and map them to APIError
	apiErr := &APIError{}

	switch {
	case errors.Is(err, tfe.ErrUnauthorized):
		apiErr.Status = 401
		apiErr.Title = "Unauthorized"
		apiErr.Detail = extractErrorDetail(err)
		return apiErr, nil

	case errors.Is(err, tfe.ErrResourceNotFound):
		apiErr.Status = 404
		apiErr.Title = "Not Found"
		apiErr.Detail = extractErrorDetail(err)
		return apiErr, nil

	default:
		// Try to parse JSON:API error body from the error message
		msg := err.Error()
		if resp := tryParseJSONAPIError(msg); resp != nil {
			// Extract status from the first error item
			if len(resp.Errors) > 0 {
				apiErr.Errors = resp.Errors
				if resp.Errors[0].Status != "" {
					// Parse status as int
					var status int
					if _, parseErr := fmt.Sscanf(resp.Errors[0].Status, "%d", &status); parseErr == nil {
						apiErr.Status = status
					}
				}
				apiErr.Title = resp.Errors[0].Title
				apiErr.Detail = resp.Errors[0].Detail
				// If status wasn't in JSON, try to detect from title/detail
				if apiErr.Status == 0 {
					apiErr.Status, _ = detectErrorType(apiErr.Title + " " + apiErr.Detail)
				}
				return apiErr, nil
			}
		}

		// Try to extract details from the error message
		// go-tfe typically wraps API errors with useful context
		detail := extractErrorDetail(err)
		if detail != "" {
			// Attempt to detect common error patterns
			status, title := detectErrorType(detail)
			apiErr.Status = status
			apiErr.Title = title
			apiErr.Detail = detail
			return apiErr, nil
		}
	}

	// Could not parse as API error
	return nil, err
}

// extractErrorDetail extracts the most useful error detail from an error chain.
func extractErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	// Try to parse any JSON:API error body in the message
	// go-tfe sometimes includes the raw response in certain errors
	if idx := strings.Index(msg, `{"errors":`); idx >= 0 {
		var resp jsonAPIErrorResponse
		if jsonErr := json.Unmarshal([]byte(msg[idx:]), &resp); jsonErr == nil && len(resp.Errors) > 0 {
			var details []string
			for _, e := range resp.Errors {
				if e.Detail != "" {
					details = append(details, e.Detail)
				} else if e.Title != "" {
					details = append(details, e.Title)
				}
			}
			if len(details) > 0 {
				return strings.Join(details, "; ")
			}
		}
	}

	return msg
}

// tryParseJSONAPIError attempts to parse a JSON:API error from an error message.
// Returns the parsed response if successful, nil otherwise.
func tryParseJSONAPIError(msg string) *jsonAPIErrorResponse {
	if idx := strings.Index(msg, `{"errors":`); idx >= 0 {
		var resp jsonAPIErrorResponse
		if err := json.Unmarshal([]byte(msg[idx:]), &resp); err == nil && len(resp.Errors) > 0 {
			return &resp
		}
	}
	return nil
}

// detectErrorType attempts to determine the error type from the error message.
func detectErrorType(detail string) (status int, title string) {
	lowerDetail := strings.ToLower(detail)

	switch {
	case strings.Contains(lowerDetail, "unauthorized") ||
		strings.Contains(lowerDetail, "authentication"):
		return 401, "Unauthorized"

	case strings.Contains(lowerDetail, "forbidden") ||
		strings.Contains(lowerDetail, "permission"):
		return 403, "Forbidden"

	case strings.Contains(lowerDetail, "not found"):
		return 404, "Not Found"

	case strings.Contains(lowerDetail, "rate limit") ||
		strings.Contains(lowerDetail, "too many requests"):
		return 429, "Rate Limited"

	case strings.Contains(lowerDetail, "service unavailable"):
		return 503, "Service Unavailable"

	case strings.Contains(lowerDetail, "internal server error"):
		return 500, "Internal Server Error"

	default:
		return 0, "Error"
	}
}

// IsRetryable returns true if the error is potentially retryable
// (e.g., rate limited or service unavailable).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	apiErr, parseErr := ParseAPIError(err)
	if parseErr != nil {
		return false
	}

	switch apiErr.Status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
