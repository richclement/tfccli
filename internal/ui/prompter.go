package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Prompter abstracts user prompts for testability.
type Prompter interface {
	// PromptString prompts for a string value with a default.
	// Returns the user's input or default if empty.
	PromptString(prompt, defaultValue string) (string, error)

	// Confirm prompts for a yes/no confirmation.
	// Returns true for yes, false for no.
	Confirm(prompt string, defaultValue bool) (bool, error)

	// PromptSelect prompts the user to select from a list of options.
	// Returns the selected value.
	PromptSelect(prompt string, options []string, defaultValue string) (string, error)
}

// StdPrompter implements Prompter using stdin/stdout.
type StdPrompter struct {
	In  io.Reader
	Out io.Writer
}

// NewStdPrompter creates a new StdPrompter using the given reader and writer.
func NewStdPrompter(in io.Reader, out io.Writer) *StdPrompter {
	return &StdPrompter{In: in, Out: out}
}

// PromptString prompts for a string value with a default.
func (p *StdPrompter) PromptString(prompt, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(p.Out, "%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Fprintf(p.Out, "%s: ", prompt)
	}

	reader := bufio.NewReader(p.In)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

// Confirm prompts for a yes/no confirmation.
func (p *StdPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	hint := "[y/N]"
	if defaultValue {
		hint = "[Y/n]"
	}
	fmt.Fprintf(p.Out, "%s %s: ", prompt, hint)

	reader := bufio.NewReader(p.In)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultValue, nil
	}

	switch input {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		// Invalid input, treat as default
		return defaultValue, nil
	}
}

// PromptSelect prompts the user to select from a list of options.
func (p *StdPrompter) PromptSelect(prompt string, options []string, defaultValue string) (string, error) {
	fmt.Fprintf(p.Out, "%s\n", prompt)
	defaultIdx := -1
	for i, opt := range options {
		marker := "  "
		if opt == defaultValue {
			marker = "> "
			defaultIdx = i
		}
		fmt.Fprintf(p.Out, "%s%d) %s\n", marker, i+1, opt)
	}

	defaultHint := ""
	if defaultIdx >= 0 {
		defaultHint = fmt.Sprintf(" [%d]", defaultIdx+1)
	}
	fmt.Fprintf(p.Out, "Enter selection%s: ", defaultHint)

	reader := bufio.NewReader(p.In)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	// Parse number selection
	var selection int
	if _, err := fmt.Sscanf(input, "%d", &selection); err == nil {
		if selection >= 1 && selection <= len(options) {
			return options[selection-1], nil
		}
	}

	// Try exact match with option name
	for _, opt := range options {
		if strings.EqualFold(input, opt) {
			return opt, nil
		}
	}

	// Default to default value on invalid input
	return defaultValue, nil
}

// ScriptedPrompter is a Prompter implementation for testing with scripted responses.
type ScriptedPrompter struct {
	stringResponses  map[string]string
	confirmResponses map[string]bool
	selectResponses  map[string]string
}

// NewScriptedPrompter creates a new scripted prompter.
func NewScriptedPrompter() *ScriptedPrompter {
	return &ScriptedPrompter{
		stringResponses:  make(map[string]string),
		confirmResponses: make(map[string]bool),
		selectResponses:  make(map[string]string),
	}
}

// OnPromptString registers a response for a PromptString call.
func (p *ScriptedPrompter) OnPromptString(prompt, response string) *ScriptedPrompter {
	p.stringResponses[prompt] = response
	return p
}

// OnConfirm registers a response for a Confirm call.
func (p *ScriptedPrompter) OnConfirm(prompt string, response bool) *ScriptedPrompter {
	p.confirmResponses[prompt] = response
	return p
}

// OnPromptSelect registers a response for a PromptSelect call.
func (p *ScriptedPrompter) OnPromptSelect(prompt, response string) *ScriptedPrompter {
	p.selectResponses[prompt] = response
	return p
}

// PromptString returns the scripted response or empty string.
func (p *ScriptedPrompter) PromptString(prompt, defaultValue string) (string, error) {
	if response, ok := p.stringResponses[prompt]; ok {
		if response == "" {
			return defaultValue, nil
		}
		return response, nil
	}
	return defaultValue, nil
}

// Confirm returns the scripted response or the default.
func (p *ScriptedPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	if response, ok := p.confirmResponses[prompt]; ok {
		return response, nil
	}
	return defaultValue, nil
}

// PromptSelect returns the scripted response or default.
func (p *ScriptedPrompter) PromptSelect(prompt string, _ []string, defaultValue string) (string, error) {
	if response, ok := p.selectResponses[prompt]; ok {
		return response, nil
	}
	return defaultValue, nil
}
