package output_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
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

// Feature: Table rendering

func TestTableWriter_DeterministicOrder(t *testing.T) {
	// Scenario: Table output is deterministic
	// When I render a table with rows in input order A,B
	// Then stdout contains rows in order A,B
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"NAME", "VALUE"}, false)
	tw.AddRow("A", "first")
	tw.AddRow("B", "second")
	tw.AddRow("C", "third")

	count, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows, got %d", count)
	}

	out := buf.String()
	// Verify order: A should appear before B, B before C
	aIdx := strings.Index(out, "A")
	bIdx := strings.Index(out, "B")
	cIdx := strings.Index(out, "C")

	if aIdx >= bIdx || bIdx >= cIdx {
		t.Errorf("rows not in input order: A@%d, B@%d, C@%d\nOutput:\n%s", aIdx, bIdx, cIdx, out)
	}
}

func TestTableWriter_NoANSIWhenNotTTY(t *testing.T) {
	// Scenario: No ANSI styling when stdout is not TTY
	// Given stdout is not a TTY
	// When I run a table command explicitly "--output-format=table"
	// Then stdout does not contain "\u001b["
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"CHECK", "STATUS"}, false) // isTTY = false
	tw.AddRow("settings", string(output.StatusPass))

	_, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "\u001b[") {
		t.Errorf("expected no ANSI escape codes when not TTY, got:\n%s", out)
	}
}

func TestTableWriter_HeadersAndSeparator(t *testing.T) {
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"ID", "NAME"}, false)
	tw.AddRow("1", "alpha")

	_, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Should have header, separator, and one data row
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d:\n%s", len(lines), out)
	}

	// Header line should contain column names
	if !strings.Contains(lines[0], "ID") || !strings.Contains(lines[0], "NAME") {
		t.Errorf("header line missing column names: %s", lines[0])
	}

	// Separator should contain dashes
	if !strings.Contains(lines[1], "-") {
		t.Errorf("separator line missing dashes: %s", lines[1])
	}
}

func TestTableWriter_ColumnAlignment(t *testing.T) {
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"NAME", "DESCRIPTION"}, false)
	tw.AddRow("a", "short")
	tw.AddRow("longervalue", "another")

	_, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// All data rows should have consistent column positions
	// The second column should start at the same position in each row
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}

	// Find where "DESCRIPTION" starts in header
	headerDescIdx := strings.Index(lines[0], "DESCRIPTION")
	// Find where "short" starts in first data row
	shortIdx := strings.Index(lines[2], "short")

	if headerDescIdx != shortIdx {
		t.Errorf("columns not aligned: header DESCRIPTION@%d, short@%d", headerDescIdx, shortIdx)
	}
}

func TestTableWriter_PadsShortRows(t *testing.T) {
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"A", "B", "C"}, false)
	tw.AddRow("only one") // Missing columns

	_, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Should not panic and should render without error
	out := buf.String()
	if !strings.Contains(out, "only one") {
		t.Errorf("expected output to contain 'only one', got:\n%s", out)
	}
}

func TestTableWriter_EmptyTable(t *testing.T) {
	var buf bytes.Buffer
	tw := output.NewTableWriter(&buf, []string{"HEADER"}, false)

	count, err := tw.Render()
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}

	out := buf.String()
	// Should still have header and separator
	if !strings.Contains(out, "HEADER") {
		t.Error("expected header even with no rows")
	}
}

// Feature: Status styling

func TestStatusStyle_NoANSIWhenNotTTY(t *testing.T) {
	// Status styling should not include ANSI when not TTY
	statuses := []output.Status{output.StatusPass, output.StatusWarn, output.StatusFail}

	for _, s := range statuses {
		result := output.StatusStyle(s, false)
		if strings.Contains(result, "\u001b[") {
			t.Errorf("StatusStyle(%s, false) should not contain ANSI, got: %s", s, result)
		}
		if result != string(s) {
			t.Errorf("StatusStyle(%s, false) expected %q, got %q", s, s, result)
		}
	}
}

func TestStatusStyle_ContainsStatusText(t *testing.T) {
	// Status text should always be present regardless of styling
	statuses := []output.Status{output.StatusPass, output.StatusWarn, output.StatusFail}

	for _, s := range statuses {
		resultTTY := output.StatusStyle(s, true)
		resultNoTTY := output.StatusStyle(s, false)

		if !strings.Contains(resultTTY, string(s)) {
			t.Errorf("StatusStyle(%s, true) should contain %q, got: %s", s, s, resultTTY)
		}
		if !strings.Contains(resultNoTTY, string(s)) {
			t.Errorf("StatusStyle(%s, false) should contain %q, got: %s", s, s, resultNoTTY)
		}
	}
}
