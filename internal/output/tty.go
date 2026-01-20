package output

import (
	"io"
	"os"

	"golang.org/x/term"
)

// TTYDetector determines whether a writer is connected to a terminal.
type TTYDetector interface {
	IsTTY(w io.Writer) bool
}

// RealTTYDetector uses golang.org/x/term for real TTY detection.
type RealTTYDetector struct{}

// IsTTY returns true if w is a *os.File connected to a terminal.
func (d *RealTTYDetector) IsTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// FakeTTYDetector always returns a fixed value (for testing).
type FakeTTYDetector struct {
	IsTTYValue bool
}

// IsTTY returns the pre-configured value.
func (d *FakeTTYDetector) IsTTY(_ io.Writer) bool {
	return d.IsTTYValue
}
