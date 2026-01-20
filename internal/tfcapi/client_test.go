package tfcapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

// newTestServer creates a test server that responds to the go-tfe ping endpoint.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-tfe pings /api/v2/ping on client creation
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Handle organizations list for Ping function tests
		if r.URL.Path == "/api/v2/organizations" {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			err := json.NewEncoder(w).Encode(map[string]any{
				"data": []any{},
			})
			if err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
			return
		}
		t.Logf("Unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestNewClient(t *testing.T) {
	// Test with a real httptest server for successful cases
	server := newTestServer(t)
	defer server.Close()

	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr string
	}{
		{
			name: "creates client with valid config",
			cfg: ClientConfig{
				Address: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			},
			wantErr: "",
		},
		{
			name: "creates client with default address when empty - uses default TFC",
			cfg: ClientConfig{
				Address: "",
				Token:   "test-token",
				Logger:  logr.Discard(),
			},
			// This will fail because it tries to connect to real app.terraform.io
			// We test this separately to verify the token-required check
			wantErr: "",
		},
		{
			name: "fails when token is empty",
			cfg: ClientConfig{
				Address: server.URL,
				Token:   "",
				Logger:  logr.Discard(),
			},
			wantErr: "token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip the empty address test for actual client creation
			// since it connects to real TFC
			if tt.cfg.Address == "" && tt.wantErr == "" {
				t.Skip("Skipping test that requires real TFC connectivity")
			}

			client, err := NewClient(tt.cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("NewClient() expected error containing %q, got nil", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("NewClient() error = %q, want error containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("NewClient() unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

func TestNewClient_TokenRequired(t *testing.T) {
	// Token validation happens before any network call
	_, err := NewClient(ClientConfig{
		Address: "https://localhost:9999",
		Token:   "",
		Logger:  logr.Discard(),
	})
	if err == nil {
		t.Error("NewClient() expected error for empty token, got nil")
		return
	}
	if !strings.Contains(err.Error(), "token is required") {
		t.Errorf("NewClient() error = %q, want error containing 'token is required'", err.Error())
	}
}

func TestNewClient_AddressNormalization(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	// The test server URL is already normalized (http://127.0.0.1:port)
	// We can't test normalization directly with httptest since it requires
	// the exact URL. Instead, we test the NormalizeAddress function separately.
	// Here we just verify the client can be created with the server URL.

	client, err := NewClient(ClientConfig{
		Address: server.URL,
		Token:   "test-token",
		Logger:  logr.Discard(),
	})
	if err != nil {
		t.Errorf("NewClient() unexpected error: %v", err)
		return
	}
	if client == nil {
		t.Error("NewClient() returned nil client")
	}
}
