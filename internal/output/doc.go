// Package output provides TTY-aware output formatting for the tfc CLI.
//
// Supports JSON and table formats with automatic detection:
//   - JSON when stdout is not a TTY (for scripting/automation)
//   - Table when stdout is a TTY (for human interaction)
//
// The --output-format flag can override the default.
package output
