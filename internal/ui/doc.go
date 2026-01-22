// Package ui provides user interaction prompts for the tfc CLI.
//
// The Prompter interface abstracts input collection for testability,
// supporting string prompts, yes/no confirmations, and selection menus.
// StdPrompter is the default implementation using stdin/stdout.
package ui
