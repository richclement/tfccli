package tfcapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	tfe "github.com/hashicorp/go-tfe"
)

// jsonAPIOrganization represents a JSON:API organization resource.
type jsonAPIOrganization struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"attributes"`
}

// jsonAPIOrganizationList represents a JSON:API organization list response.
type jsonAPIOrganizationList struct {
	Data []jsonAPIOrganization `json:"data"`
	Meta struct {
		Pagination struct {
			CurrentPage int `json:"current-page"`
			NextPage    int `json:"next-page"`
			PrevPage    int `json:"prev-page"`
			TotalPages  int `json:"total-pages"`
			TotalCount  int `json:"total-count"`
		} `json:"pagination"`
	} `json:"meta"`
}

func TestCollectAllOrganizations_AggregatesMultiplePages(t *testing.T) {
	// Create test server that returns paginated responses
	orgRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		orgRequestCount++

		pageNum := r.URL.Query().Get("page[number]")
		if pageNum == "" {
			pageNum = "1"
		}
		page, _ := strconv.Atoi(pageNum)

		var resp jsonAPIOrganizationList

		switch page {
		case 1:
			resp = jsonAPIOrganizationList{
				Data: []jsonAPIOrganization{
					{ID: "org-1", Type: "organizations", Attributes: struct {
						Name  string `json:"name"`
						Email string `json:"email"`
					}{Name: "org-one", Email: "org1@example.com"}},
					{ID: "org-2", Type: "organizations", Attributes: struct {
						Name  string `json:"name"`
						Email string `json:"email"`
					}{Name: "org-two", Email: "org2@example.com"}},
				},
			}
			resp.Meta.Pagination.CurrentPage = 1
			resp.Meta.Pagination.NextPage = 2
			resp.Meta.Pagination.TotalPages = 2
			resp.Meta.Pagination.TotalCount = 3
		case 2:
			resp = jsonAPIOrganizationList{
				Data: []jsonAPIOrganization{
					{ID: "org-3", Type: "organizations", Attributes: struct {
						Name  string `json:"name"`
						Email string `json:"email"`
					}{Name: "org-three", Email: "org3@example.com"}},
				},
			}
			resp.Meta.Pagination.CurrentPage = 2
			resp.Meta.Pagination.NextPage = 0 // No more pages
			resp.Meta.Pagination.TotalPages = 2
			resp.Meta.Pagination.TotalCount = 3
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client pointing to test server
	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test aggregation
	orgs, err := CollectAllOrganizations(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("CollectAllOrganizations failed: %v", err)
	}

	// Verify all 3 items were aggregated
	if len(orgs) != 3 {
		t.Errorf("expected 3 organizations, got %d", len(orgs))
	}

	// Verify both pages were requested
	if orgRequestCount != 2 {
		t.Errorf("expected 2 requests, got %d", orgRequestCount)
	}
}

func TestCollectAllOrganizations_StopsOnEmptyPage(t *testing.T) {
	orgRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		orgRequestCount++

		pageNum := r.URL.Query().Get("page[number]")
		if pageNum == "" {
			pageNum = "1"
		}
		page, _ := strconv.Atoi(pageNum)

		var resp jsonAPIOrganizationList

		switch page {
		case 1:
			resp = jsonAPIOrganizationList{
				Data: []jsonAPIOrganization{
					{ID: "org-1", Type: "organizations", Attributes: struct {
						Name  string `json:"name"`
						Email string `json:"email"`
					}{Name: "org-one", Email: "org1@example.com"}},
				},
			}
			resp.Meta.Pagination.CurrentPage = 1
			resp.Meta.Pagination.NextPage = 2
			resp.Meta.Pagination.TotalPages = 2
			resp.Meta.Pagination.TotalCount = 1
		case 2:
			// Empty page
			resp = jsonAPIOrganizationList{
				Data: []jsonAPIOrganization{},
			}
			resp.Meta.Pagination.CurrentPage = 2
			resp.Meta.Pagination.NextPage = 3 // Would continue but we stop on empty
			resp.Meta.Pagination.TotalPages = 3
		default:
			t.Errorf("unexpected page %d requested", page)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	orgs, err := CollectAllOrganizations(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("CollectAllOrganizations failed: %v", err)
	}

	// Should only have 1 item (from page 1)
	if len(orgs) != 1 {
		t.Errorf("expected 1 organization, got %d", len(orgs))
	}

	// Should stop after empty page (page 2) and not request page 3
	if orgRequestCount != 2 {
		t.Errorf("expected 2 requests (stop on empty), got %d", orgRequestCount)
	}
}

func TestCollectAllOrganizations_StopsWhenNextPageIsZero(t *testing.T) {
	orgRequestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		orgRequestCount++

		resp := jsonAPIOrganizationList{
			Data: []jsonAPIOrganization{
				{ID: "org-1", Type: "organizations", Attributes: struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				}{Name: "org-one", Email: "org1@example.com"}},
			},
		}
		resp.Meta.Pagination.CurrentPage = 1
		resp.Meta.Pagination.NextPage = 0 // No more pages
		resp.Meta.Pagination.TotalPages = 1
		resp.Meta.Pagination.TotalCount = 1

		w.Header().Set("Content-Type", "application/vnd.api+json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	orgs, err := CollectAllOrganizations(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("CollectAllOrganizations failed: %v", err)
	}

	if len(orgs) != 1 {
		t.Errorf("expected 1 organization, got %d", len(orgs))
	}

	// Should only make 1 request
	if orgRequestCount != 1 {
		t.Errorf("expected 1 request, got %d", orgRequestCount)
	}
}

func TestCollectAllOrganizations_RespectsPageSize(t *testing.T) {
	requestedPageSize := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		requestedPageSize, _ = strconv.Atoi(r.URL.Query().Get("page[size]"))

		resp := jsonAPIOrganizationList{
			Data: []jsonAPIOrganization{},
		}
		resp.Meta.Pagination.NextPage = 0

		w.Header().Set("Content-Type", "application/vnd.api+json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Custom page size
	opts := &tfe.OrganizationListOptions{}
	opts.PageSize = 50
	_, err = CollectAllOrganizations(context.Background(), client, opts)
	if err != nil {
		t.Fatalf("CollectAllOrganizations failed: %v", err)
	}

	if requestedPageSize != 50 {
		t.Errorf("expected page size 50, got %d", requestedPageSize)
	}
}

func TestCollectAllOrganizations_UsesDefaultPageSize(t *testing.T) {
	requestedPageSize := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		requestedPageSize, _ = strconv.Atoi(r.URL.Query().Get("page[size]"))

		resp := jsonAPIOrganizationList{
			Data: []jsonAPIOrganization{},
		}
		resp.Meta.Pagination.NextPage = 0

		w.Header().Set("Content-Type", "application/vnd.api+json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// No page size specified
	_, err = CollectAllOrganizations(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("CollectAllOrganizations failed: %v", err)
	}

	if requestedPageSize != DefaultPageSize {
		t.Errorf("expected default page size %d, got %d", DefaultPageSize, requestedPageSize)
	}
}

func TestCollectAllOrganizations_PropagatesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe sends a /api/v2/ping request when creating client
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"status":"500","title":"Internal Server Error"]}}`)) //nolint:errcheck
	}))
	defer server.Close()

	cfg := &tfe.Config{
		Address:           server.URL,
		Token:             "test-token",
		RetryServerErrors: false,
	}
	client, err := tfe.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = CollectAllOrganizations(context.Background(), client, nil)
	if err == nil {
		t.Error("expected error, got nil")
	}
}
