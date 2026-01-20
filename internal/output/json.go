package output

import (
	"encoding/json"
	"io"
)

// WriteJSON writes data as JSON to the writer.
// For JSON:API passthrough, data should be the raw API response.
func WriteJSON(w io.Writer, data any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// EmptySuccessResponse represents the response for 204/empty-body successes.
type EmptySuccessResponse struct {
	Meta EmptyMeta `json:"meta"`
}

// EmptyMeta contains status information for empty responses.
type EmptyMeta struct {
	Status int `json:"status"`
}

// WriteEmptySuccess writes a standard JSON response for empty-body successes (e.g., 204 No Content).
// Output: {"meta":{"status":204}}
func WriteEmptySuccess(w io.Writer, statusCode int) error {
	response := EmptySuccessResponse{
		Meta: EmptyMeta{Status: statusCode},
	}
	return WriteJSON(w, response)
}
