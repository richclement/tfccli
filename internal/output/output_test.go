package output_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/richclement/tfccli/internal/output"
)

// Feature: Output format defaults

func TestResolveOutputFormat_DefaultsToTableOnTTY(t *testing.T) {
	// Scenario: Defaults to table on TTY
	// Given stdout is a TTY
	isTTY := true
	// When I resolve output format with no flag value
	format := output.ResolveOutputFormat("", isTTY)
	// Then effective output format = "table"
	if format != output.FormatTable {
		t.Errorf("expected table, got %s", format)
	}
}

func TestResolveOutputFormat_DefaultsToJSONWhenNotTTY(t *testing.T) {
	// Scenario: Defaults to json when stdout is not a TTY
	// Given stdout is not a TTY
	isTTY := false
	// When I resolve output format with no flag value
	format := output.ResolveOutputFormat("", isTTY)
	// Then effective output format = "json"
	if format != output.FormatJSON {
		t.Errorf("expected json, got %s", format)
	}
}

func TestResolveOutputFormat_FlagOverridesTTY(t *testing.T) {
	// Flag value takes precedence over TTY detection
	tests := []struct {
		name     string
		flagVal  string
		isTTY    bool
		expected output.Format
	}{
		{"json flag on TTY", "json", true, output.FormatJSON},
		{"json flag not TTY", "json", false, output.FormatJSON},
		{"table flag on TTY", "table", true, output.FormatTable},
		{"table flag not TTY", "table", false, output.FormatTable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := output.ResolveOutputFormat(tt.flagVal, tt.isTTY)
			if format != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, format)
			}
		})
	}
}

// Feature: TTY Detection

func TestRealTTYDetector_IsTTY_FileNotTerminal(t *testing.T) {
	// A bytes.Buffer is not a terminal
	detector := &output.RealTTYDetector{}
	var buf bytes.Buffer
	if detector.IsTTY(&buf) {
		t.Error("expected bytes.Buffer to not be a TTY")
	}
}

func TestRealTTYDetector_IsTTY_DevNull(t *testing.T) {
	// /dev/null is a file but not a terminal
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer f.Close()

	detector := &output.RealTTYDetector{}
	if detector.IsTTY(f) {
		t.Errorf("expected %s to not be a TTY", os.DevNull)
	}
}

func TestFakeTTYDetector_ReturnsPreconfiguredValue(t *testing.T) {
	var buf bytes.Buffer

	detectorTrue := &output.FakeTTYDetector{IsTTYValue: true}
	if !detectorTrue.IsTTY(&buf) {
		t.Error("expected FakeTTYDetector with IsTTYValue=true to return true")
	}

	detectorFalse := &output.FakeTTYDetector{IsTTYValue: false}
	if detectorFalse.IsTTY(&buf) {
		t.Error("expected FakeTTYDetector with IsTTYValue=false to return false")
	}
}

// Feature: JSON emitter

func TestWriteJSON_EmitsValidJSON(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	var buf bytes.Buffer

	err := output.WriteJSON(&buf, data)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", result)
	}
}

func TestWriteJSON_JSONAPIPassthrough(t *testing.T) {
	// Simulates raw JSON:API response passthrough
	jsonAPI := map[string]any{
		"data": map[string]any{
			"type": "organizations",
			"id":   "org-123",
			"attributes": map[string]any{
				"name": "acme",
			},
		},
	}
	var buf bytes.Buffer

	err := output.WriteJSON(&buf, jsonAPI)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Verify structure preserved
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data field")
	}
	if data["type"] != "organizations" {
		t.Errorf("expected type=organizations, got %v", data["type"])
	}
}

func TestWriteEmptySuccess_204NoContent(t *testing.T) {
	// Scenario: Empty-body success emits meta JSON in json mode
	// Given API returns 204 No Content
	// When I run a delete command with --force
	// Then stdout parses as JSON
	// And stdout.meta.status = 204
	var buf bytes.Buffer

	err := output.WriteEmptySuccess(&buf, 204)
	if err != nil {
		t.Fatalf("WriteEmptySuccess failed: %v", err)
	}

	var result output.EmptySuccessResponse
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if result.Meta.Status != 204 {
		t.Errorf("expected meta.status=204, got %d", result.Meta.Status)
	}
}

func TestWriteEmptySuccess_OtherStatusCodes(t *testing.T) {
	// Test other empty-body status codes
	statusCodes := []int{200, 201, 202, 204, 304}

	for _, code := range statusCodes {
		var buf bytes.Buffer
		err := output.WriteEmptySuccess(&buf, code)
		if err != nil {
			t.Fatalf("WriteEmptySuccess(%d) failed: %v", code, err)
		}

		var result output.EmptySuccessResponse
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("output for %d is not valid JSON: %v", code, err)
		}

		if result.Meta.Status != code {
			t.Errorf("expected meta.status=%d, got %d", code, result.Meta.Status)
		}
	}
}
