package testutil

import (
	"errors"

	"github.com/richclement/tfccli/internal/ui"
)

// Verify interface compliance at compile time.
var (
	_ ui.Prompter = (*AcceptingPrompter)(nil)
	_ ui.Prompter = (*RejectingPrompter)(nil)
	_ ui.Prompter = (*FailingPrompter)(nil)
)

// AcceptingPrompter always returns true for confirms and default values for other prompts.
// Use this when testing the "user confirms" path.
type AcceptingPrompter struct{}

// PromptString returns the default value.
func (p *AcceptingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

// Confirm always returns true.
func (p *AcceptingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return true, nil
}

// PromptSelect returns the default value.
func (p *AcceptingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// RejectingPrompter always returns false for confirms and default values for other prompts.
// Use this when testing the "user declines" path.
type RejectingPrompter struct{}

// PromptString returns the default value.
func (p *RejectingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

// Confirm always returns false.
func (p *RejectingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, nil
}

// PromptSelect returns the default value.
func (p *RejectingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// FailingPrompter returns an error for all prompts.
// Use this to verify that --force bypasses prompts (the test will fail if
// the prompter is called).
type FailingPrompter struct{}

// ErrPrompterShouldNotBeCalled is returned by FailingPrompter methods.
var ErrPrompterShouldNotBeCalled = errors.New("prompter should not be called (use --force)")

// PromptString returns an error.
func (p *FailingPrompter) PromptString(_, _ string) (string, error) {
	return "", ErrPrompterShouldNotBeCalled
}

// Confirm returns an error.
func (p *FailingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, ErrPrompterShouldNotBeCalled
}

// PromptSelect returns an error.
func (p *FailingPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
	return "", ErrPrompterShouldNotBeCalled
}
