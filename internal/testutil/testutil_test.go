package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/richclement/tfccli/internal/config"
)

// TestTempHome_CreatesDirectory verifies TempHome creates a temp directory.
func TestTempHome_CreatesDirectory(t *testing.T) {
	tmpDir := TempHome(t, nil)

	if tmpDir == "" {
		t.Error("expected non-empty temp directory path")
	}

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("temp directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected temp path to be a directory")
	}
}

// TestTempHome_WithSettings verifies settings are written.
func TestTempHome_WithSettings(t *testing.T) {
	settings := DefaultTestSettings()
	tmpDir := TempHome(t, settings)

	// Verify settings.json exists
	settingsPath := filepath.Join(tmpDir, ".tfccli", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// Load and verify
	loaded, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}
	if loaded.CurrentContext != "default" {
		t.Errorf("expected current_context 'default', got %q", loaded.CurrentContext)
	}
}

// TestTempHome_Isolation verifies the real home is not modified.
func TestTempHome_Isolation(t *testing.T) {
	realHome, _ := os.UserHomeDir()

	settings := DefaultTestSettings()
	tmpDir := TempHome(t, settings)

	// Temp dir should not be the real home
	if tmpDir == realHome {
		t.Error("temp directory should not be the real home directory")
	}

	// Settings should be in tmpDir, not realHome
	settingsInTmp := filepath.Join(tmpDir, ".tfccli", "settings.json")
	if _, err := os.Stat(settingsInTmp); err != nil {
		t.Fatalf("settings not in temp dir: %v", err)
	}
}

// TestRequestRecorder_CapturesRequests verifies requests are recorded.
func TestRequestRecorder_CapturesRequests(t *testing.T) {
	recorder := NewRequestRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.Record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make a POST request with body
	reqBody := strings.NewReader(`{"key":"value"}`)
	req, _ := http.NewRequest("POST", server.URL+"/api/v2/test?foo=bar", reqBody)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Verify recording
	if recorder.Count() != 1 {
		t.Errorf("expected 1 recorded request, got %d", recorder.Count())
	}

	recorded := recorder.Last()
	if recorded == nil {
		t.Fatal("expected recorded request")
	}

	if recorded.Method != "POST" {
		t.Errorf("expected method POST, got %s", recorded.Method)
	}
	if recorded.Path != "/api/v2/test" {
		t.Errorf("expected path /api/v2/test, got %s", recorded.Path)
	}
	if recorded.Query.Get("foo") != "bar" {
		t.Errorf("expected query foo=bar, got %s", recorded.Query.Get("foo"))
	}
	if !recorded.HasAuthorizationHeader() {
		t.Error("expected Authorization header")
	}
	if recorded.BodyString() != `{"key":"value"}` {
		t.Errorf("expected body, got %s", recorded.BodyString())
	}
}

// TestRequestRecorder_HasRequest verifies HasRequest helper.
func TestRequestRecorder_HasRequest(t *testing.T) {
	recorder := NewRequestRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.Record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make GET request
	resp, _ := http.Get(server.URL + "/api/v2/orgs")
	resp.Body.Close()

	// Make POST request
	resp, _ = http.Post(server.URL+"/api/v2/workspaces", "application/json", nil)
	resp.Body.Close()

	if !recorder.HasRequest("GET", "/api/v2/orgs") {
		t.Error("expected HasRequest to find GET /api/v2/orgs")
	}
	if !recorder.HasRequest("POST", "/api/v2/workspaces") {
		t.Error("expected HasRequest to find POST /api/v2/workspaces")
	}
	if recorder.HasRequest("DELETE", "/api/v2/orgs") {
		t.Error("expected HasRequest to not find DELETE /api/v2/orgs")
	}
}

// TestRequestRecorder_First verifies First helper.
func TestRequestRecorder_First(t *testing.T) {
	recorder := NewRequestRecorder()

	// First on empty recorder returns nil
	if recorder.First() != nil {
		t.Error("expected First() to return nil on empty recorder")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.Record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make GET request first
	resp, _ := http.Get(server.URL + "/first")
	resp.Body.Close()

	// Make POST request second
	resp, _ = http.Post(server.URL+"/second", "application/json", nil)
	resp.Body.Close()

	first := recorder.First()
	if first == nil {
		t.Fatal("expected First() to return a request")
	}
	if first.Path != "/first" {
		t.Errorf("expected first request path /first, got %s", first.Path)
	}
	if first.Method != "GET" {
		t.Errorf("expected first request method GET, got %s", first.Method)
	}
}

// TestRequestRecorder_RequestsForPath verifies path filtering.
func TestRequestRecorder_RequestsForPath(t *testing.T) {
	recorder := NewRequestRecorder()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.Record(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Make multiple requests to same path
	for range 3 {
		resp, _ := http.Get(server.URL + "/api/v2/orgs")
		resp.Body.Close()
	}
	resp, _ := http.Get(server.URL + "/api/v2/other")
	resp.Body.Close()

	orgsRequests := recorder.RequestsForPath("/api/v2/orgs")
	if len(orgsRequests) != 3 {
		t.Errorf("expected 3 requests to /api/v2/orgs, got %d", len(orgsRequests))
	}
}

// TestAcceptingPrompter_ConfirmReturnsTrue verifies AcceptingPrompter.
func TestAcceptingPrompter_ConfirmReturnsTrue(t *testing.T) {
	p := &AcceptingPrompter{}

	confirmed, err := p.Confirm("Delete resource?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected AcceptingPrompter to return true")
	}
}

// TestRejectingPrompter_ConfirmReturnsFalse verifies RejectingPrompter.
func TestRejectingPrompter_ConfirmReturnsFalse(t *testing.T) {
	p := &RejectingPrompter{}

	confirmed, err := p.Confirm("Delete resource?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Error("expected RejectingPrompter to return false")
	}
}

// TestFailingPrompter_ReturnsError verifies FailingPrompter.
func TestFailingPrompter_ReturnsError(t *testing.T) {
	p := &FailingPrompter{}

	_, err := p.Confirm("Delete?", false)
	if err == nil {
		t.Error("expected FailingPrompter to return error")
	}
	if err != ErrPrompterShouldNotBeCalled {
		t.Errorf("expected ErrPrompterShouldNotBeCalled, got %v", err)
	}

	_, err = p.PromptString("Name:", "default")
	if err != ErrPrompterShouldNotBeCalled {
		t.Errorf("expected ErrPrompterShouldNotBeCalled, got %v", err)
	}

	_, err = p.PromptSelect("Choose:", []string{"a", "b"}, "a")
	if err != ErrPrompterShouldNotBeCalled {
		t.Errorf("expected ErrPrompterShouldNotBeCalled, got %v", err)
	}
}

// TestFakeEnv_GetenvReturnsValue verifies FakeEnv.
func TestFakeEnv_GetenvReturnsValue(t *testing.T) {
	env := NewFakeEnv(map[string]string{
		"TF_TOKEN_app_terraform_io": "test-token",
		"HOME":                      "/tmp/test",
	})

	if got := env.Getenv("TF_TOKEN_app_terraform_io"); got != "test-token" {
		t.Errorf("expected 'test-token', got %q", got)
	}
	if got := env.Getenv("MISSING"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}

	env.Set("NEW_VAR", "new-value")
	if got := env.Getenv("NEW_VAR"); got != "new-value" {
		t.Errorf("expected 'new-value', got %q", got)
	}
}

// TestFakeFS_ReadFileReturnsContent verifies FakeFS.
func TestFakeFS_ReadFileReturnsContent(t *testing.T) {
	fs := NewFakeFS("/home/test", map[string][]byte{
		"/home/test/.terraform.d/credentials.tfrc.json": []byte(`{"credentials":{}}`),
	})

	data, err := fs.ReadFile("/home/test/.terraform.d/credentials.tfrc.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"credentials":{}}` {
		t.Errorf("unexpected content: %s", string(data))
	}

	_, err = fs.ReadFile("/missing/file")
	if err == nil {
		t.Error("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}

	homeDir, _ := fs.UserHomeDir()
	if homeDir != "/home/test" {
		t.Errorf("expected home dir /home/test, got %q", homeDir)
	}
}

// TestTokenResolverWithEnvToken verifies the helper creates a working resolver.
func TestTokenResolverWithEnvToken(t *testing.T) {
	resolver := TokenResolverWithEnvToken("/tmp/test", "app.terraform.io", "my-token")

	result, err := resolver.ResolveToken("app.terraform.io")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token != "my-token" {
		t.Errorf("expected token 'my-token', got %q", result.Token)
	}
	if result.Source != "env" {
		t.Errorf("expected source 'env', got %q", result.Source)
	}
}

// TestRequestRecorder_BodyPreserved verifies body is still available to handler.
func TestRequestRecorder_BodyPreserved(t *testing.T) {
	recorder := NewRequestRecorder()
	var handlerBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.Record(r)
		// Handler should still be able to read body after recording
		handlerBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reqBody := strings.NewReader(`{"test":"data"}`)
	resp, _ := http.Post(server.URL+"/test", "application/json", reqBody)
	resp.Body.Close()

	// Both recorder and handler should have the body
	if recorder.Last().BodyString() != `{"test":"data"}` {
		t.Errorf("recorder didn't capture body: %s", recorder.Last().BodyString())
	}
	if string(handlerBody) != `{"test":"data"}` {
		t.Errorf("handler didn't receive body: %s", string(handlerBody))
	}
}

// TestDefaultTestSettings verifies the default settings helper.
func TestDefaultTestSettings(t *testing.T) {
	settings := DefaultTestSettings()

	if settings.CurrentContext != "default" {
		t.Errorf("expected current_context 'default', got %q", settings.CurrentContext)
	}
	if ctx, ok := settings.Contexts["default"]; !ok {
		t.Error("expected 'default' context")
	} else {
		if ctx.Address != "app.terraform.io" {
			t.Errorf("expected address 'app.terraform.io', got %q", ctx.Address)
		}
		if ctx.DefaultOrg != "test-org" {
			t.Errorf("expected default_org 'test-org', got %q", ctx.DefaultOrg)
		}
	}
}

// TestMultiContextSettings verifies the multi-context settings helper.
func TestMultiContextSettings(t *testing.T) {
	settings := MultiContextSettings()

	if len(settings.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(settings.Contexts))
	}
	if _, ok := settings.Contexts["default"]; !ok {
		t.Error("expected 'default' context")
	}
	if _, ok := settings.Contexts["prod"]; !ok {
		t.Error("expected 'prod' context")
	}
}
