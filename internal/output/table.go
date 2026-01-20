package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/muesli/termenv"
)

// TableWriter renders tabular output with optional styling.
type TableWriter struct {
	w          io.Writer
	headers    []string
	rows       [][]string
	colWidths  []int
	profile    termenv.Profile
	styleColor bool
}

// NewTableWriter creates a table writer.
// If isTTY is false, ANSI styling is disabled.
func NewTableWriter(w io.Writer, headers []string, isTTY bool) *TableWriter {
	tw := &TableWriter{
		w:         w,
		headers:   headers,
		rows:      make([][]string, 0),
		colWidths: make([]int, len(headers)),
	}

	// Initialize column widths from headers
	for i, h := range headers {
		tw.colWidths[i] = len(h)
	}

	// Set termenv profile based on TTY
	if isTTY {
		tw.profile = termenv.ColorProfile()
		tw.styleColor = true
	} else {
		tw.profile = termenv.Ascii
		tw.styleColor = false
	}

	return tw
}

// AddRow adds a row of values to the table.
// The number of values should match the number of headers.
func (tw *TableWriter) AddRow(values ...string) {
	// Pad or truncate to match header count
	row := make([]string, len(tw.headers))
	for i := range row {
		if i < len(values) {
			row[i] = values[i]
		}
	}

	// Update column widths
	for i, v := range row {
		if len(v) > tw.colWidths[i] {
			tw.colWidths[i] = len(v)
		}
	}

	tw.rows = append(tw.rows, row)
}

// Render writes the table to the writer.
// Returns the number of rows (excluding header).
func (tw *TableWriter) Render() (int, error) {
	// Render header
	headerLine := tw.formatRow(tw.headers, true)
	if _, err := fmt.Fprintln(tw.w, headerLine); err != nil {
		return 0, err
	}

	// Render separator
	sep := tw.separator()
	if _, err := fmt.Fprintln(tw.w, sep); err != nil {
		return 0, err
	}

	// Render rows in order
	for _, row := range tw.rows {
		line := tw.formatRow(row, false)
		if _, err := fmt.Fprintln(tw.w, line); err != nil {
			return len(tw.rows), err
		}
	}

	return len(tw.rows), nil
}

// formatRow formats a single row with proper column spacing.
func (tw *TableWriter) formatRow(values []string, isHeader bool) string {
	parts := make([]string, len(values))
	for i, v := range values {
		padded := fmt.Sprintf("%-*s", tw.colWidths[i], v)
		if isHeader && tw.styleColor {
			// Bold headers when styling is enabled
			padded = termenv.String(padded).Bold().String()
		}
		parts[i] = padded
	}
	return strings.Join(parts, "  ")
}

// separator creates a line of dashes under the header.
func (tw *TableWriter) separator() string {
	parts := make([]string, len(tw.colWidths))
	for i, w := range tw.colWidths {
		parts[i] = strings.Repeat("-", w)
	}
	return strings.Join(parts, "  ")
}

// Status represents a check status for doctor output.
type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

// StatusStyle returns a styled status string.
// If isTTY is false, returns the plain status string.
func StatusStyle(status Status, isTTY bool) string {
	s := string(status)
	if !isTTY {
		return s
	}

	style := termenv.String(s)
	switch status {
	case StatusPass:
		style = style.Foreground(termenv.ANSIGreen)
	case StatusWarn:
		style = style.Foreground(termenv.ANSIYellow)
	case StatusFail:
		style = style.Foreground(termenv.ANSIRed)
	}
	return style.String()
}
