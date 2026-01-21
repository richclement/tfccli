package main

import "errors"

// acceptingPrompter always returns true for confirms.
type acceptingPrompter struct{}

func (p *acceptingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *acceptingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return true, nil
}

func (p *acceptingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// rejectingPrompter always returns false for confirms.
type rejectingPrompter struct{}

func (p *rejectingPrompter) PromptString(_, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (p *rejectingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, nil
}

func (p *rejectingPrompter) PromptSelect(_ string, _ []string, defaultValue string) (string, error) {
	return defaultValue, nil
}

// failingPrompter returns an error to verify prompts are bypassed with --force.
type failingPrompter struct{}

func (p *failingPrompter) PromptString(_, _ string) (string, error) {
	return "", errors.New("should not be called with --force")
}

func (p *failingPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, errors.New("should not be called with --force")
}

func (p *failingPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
	return "", errors.New("should not be called with --force")
}

// errorPrompter returns a configurable error for testing prompter error paths.
type errorPrompter struct {
	err error
}

func (p *errorPrompter) PromptString(_, _ string) (string, error) {
	return "", p.err
}

func (p *errorPrompter) Confirm(_ string, _ bool) (bool, error) {
	return false, p.err
}

func (p *errorPrompter) PromptSelect(_ string, _ []string, _ string) (string, error) {
	return "", p.err
}
