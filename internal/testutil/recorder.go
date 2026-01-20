package testutil

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sync"
)

// RecordedRequest captures details of an HTTP request for assertions.
type RecordedRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Headers http.Header
	Body    []byte
}

// RequestRecorder captures HTTP requests for test assertions.
// It is safe for concurrent use.
type RequestRecorder struct {
	mu       sync.Mutex
	requests []RecordedRequest
}

// NewRequestRecorder creates a new RequestRecorder.
func NewRequestRecorder() *RequestRecorder {
	return &RequestRecorder{}
}

// Record captures the details of an HTTP request.
// Call this in your httptest handler to record requests.
func (r *RequestRecorder) Record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Read and store body, but also restore it for the handler
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	r.requests = append(r.requests, RecordedRequest{
		Method:  req.Method,
		Path:    req.URL.Path,
		Query:   req.URL.Query(),
		Headers: req.Header.Clone(),
		Body:    body,
	})
}

// Requests returns all recorded requests.
func (r *RequestRecorder) Requests() []RecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]RecordedRequest, len(r.requests))
	copy(result, r.requests)
	return result
}

// Count returns the number of recorded requests.
func (r *RequestRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// Last returns the most recent request, or nil if none recorded.
func (r *RequestRecorder) Last() *RecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.requests) == 0 {
		return nil
	}
	req := r.requests[len(r.requests)-1]
	return &req
}

// Clear removes all recorded requests.
func (r *RequestRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}

// HasRequest returns true if any request matches the given method and path.
func (r *RequestRecorder) HasRequest(method, path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, req := range r.requests {
		if req.Method == method && req.Path == path {
			return true
		}
	}
	return false
}

// RequestsForPath returns all requests matching the given path.
func (r *RequestRecorder) RequestsForPath(path string) []RecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []RecordedRequest
	for _, req := range r.requests {
		if req.Path == path {
			result = append(result, req)
		}
	}
	return result
}

// HasAuthorizationHeader returns true if the request has an Authorization header.
func (req *RecordedRequest) HasAuthorizationHeader() bool {
	return req.Headers.Get("Authorization") != ""
}

// GetAuthorizationHeader returns the Authorization header value.
func (req *RecordedRequest) GetAuthorizationHeader() string {
	return req.Headers.Get("Authorization")
}

// BodyString returns the request body as a string.
func (req *RecordedRequest) BodyString() string {
	return string(req.Body)
}
