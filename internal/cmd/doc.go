// Package cmd provides shared command utilities for the tfc CLI.
//
// The primary type is RuntimeError, which wraps errors to signal exit code 2
// (runtime error) as opposed to exit code 1 (usage error) or 3 (internal error).
package cmd
