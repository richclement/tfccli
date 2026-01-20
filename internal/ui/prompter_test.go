package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestStdPrompter_PromptString(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		prompt       string
		defaultValue string
		want         string
		wantOutput   string
	}{
		{
			name:         "returns user input",
			input:        "myvalue\n",
			prompt:       "Enter value",
			defaultValue: "default",
			want:         "myvalue",
			wantOutput:   "Enter value [default]: ",
		},
		{
			name:         "returns default on empty input",
			input:        "\n",
			prompt:       "Enter value",
			defaultValue: "default",
			want:         "default",
			wantOutput:   "Enter value [default]: ",
		},
		{
			name:         "no default value shown when empty",
			input:        "test\n",
			prompt:       "Enter value",
			defaultValue: "",
			want:         "test",
			wantOutput:   "Enter value: ",
		},
		{
			name:         "trims whitespace from input",
			input:        "  value  \n",
			prompt:       "Enter value",
			defaultValue: "",
			want:         "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdPrompter(in, out)

			got, err := p.PromptString(tt.prompt, tt.defaultValue)
			if err != nil {
				t.Fatalf("PromptString() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("PromptString() = %q, want %q", got, tt.want)
			}
			if tt.wantOutput != "" && !strings.Contains(out.String(), tt.wantOutput) {
				t.Errorf("output = %q, want to contain %q", out.String(), tt.wantOutput)
			}
		})
	}
}

func TestStdPrompter_Confirm(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		prompt       string
		defaultValue bool
		want         bool
		wantHint     string
	}{
		{
			name:         "y returns true",
			input:        "y\n",
			prompt:       "Continue?",
			defaultValue: false,
			want:         true,
			wantHint:     "[y/N]",
		},
		{
			name:         "yes returns true",
			input:        "yes\n",
			prompt:       "Continue?",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "Y returns true (case insensitive)",
			input:        "Y\n",
			prompt:       "Continue?",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "n returns false",
			input:        "n\n",
			prompt:       "Continue?",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "no returns false",
			input:        "no\n",
			prompt:       "Continue?",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "empty input returns default false",
			input:        "\n",
			prompt:       "Continue?",
			defaultValue: false,
			want:         false,
			wantHint:     "[y/N]",
		},
		{
			name:         "empty input returns default true",
			input:        "\n",
			prompt:       "Continue?",
			defaultValue: true,
			want:         true,
			wantHint:     "[Y/n]",
		},
		{
			name:         "invalid input returns default",
			input:        "maybe\n",
			prompt:       "Continue?",
			defaultValue: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdPrompter(in, out)

			got, err := p.Confirm(tt.prompt, tt.defaultValue)
			if err != nil {
				t.Fatalf("Confirm() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Confirm() = %v, want %v", got, tt.want)
			}
			if tt.wantHint != "" && !strings.Contains(out.String(), tt.wantHint) {
				t.Errorf("output = %q, want to contain hint %q", out.String(), tt.wantHint)
			}
		})
	}
}

func TestStdPrompter_PromptSelect(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		prompt       string
		options      []string
		defaultValue string
		want         string
	}{
		{
			name:         "numeric selection returns option",
			input:        "2\n",
			prompt:       "Select option",
			options:      []string{"alpha", "beta", "gamma"},
			defaultValue: "alpha",
			want:         "beta",
		},
		{
			name:         "empty input returns default",
			input:        "\n",
			prompt:       "Select option",
			options:      []string{"alpha", "beta", "gamma"},
			defaultValue: "beta",
			want:         "beta",
		},
		{
			name:         "exact match returns option",
			input:        "gamma\n",
			prompt:       "Select option",
			options:      []string{"alpha", "beta", "gamma"},
			defaultValue: "alpha",
			want:         "gamma",
		},
		{
			name:         "case insensitive match",
			input:        "BETA\n",
			prompt:       "Select option",
			options:      []string{"alpha", "beta", "gamma"},
			defaultValue: "alpha",
			want:         "beta",
		},
		{
			name:         "invalid selection returns default",
			input:        "99\n",
			prompt:       "Select option",
			options:      []string{"alpha", "beta"},
			defaultValue: "alpha",
			want:         "alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdPrompter(in, out)

			got, err := p.PromptSelect(tt.prompt, tt.options, tt.defaultValue)
			if err != nil {
				t.Fatalf("PromptSelect() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("PromptSelect() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScriptedPrompter_PromptString(t *testing.T) {
	p := NewScriptedPrompter().
		OnPromptString("Name", "Alice").
		OnPromptString("Empty", "")

	// Scripted response
	got, err := p.PromptString("Name", "default")
	if err != nil {
		t.Fatalf("PromptString() error = %v", err)
	}
	if got != "Alice" {
		t.Errorf("PromptString() = %q, want %q", got, "Alice")
	}

	// Empty scripted response returns default
	got, err = p.PromptString("Empty", "fallback")
	if err != nil {
		t.Fatalf("PromptString() error = %v", err)
	}
	if got != "fallback" {
		t.Errorf("PromptString() = %q, want %q", got, "fallback")
	}

	// Unknown prompt returns default
	got, err = p.PromptString("Unknown", "thedefault")
	if err != nil {
		t.Fatalf("PromptString() error = %v", err)
	}
	if got != "thedefault" {
		t.Errorf("PromptString() = %q, want %q", got, "thedefault")
	}
}

func TestScriptedPrompter_Confirm(t *testing.T) {
	p := NewScriptedPrompter().
		OnConfirm("Delete?", true).
		OnConfirm("Abort?", false)

	// Scripted true
	got, _ := p.Confirm("Delete?", false)
	if got != true {
		t.Errorf("Confirm() = %v, want true", got)
	}

	// Scripted false
	got, _ = p.Confirm("Abort?", true)
	if got != false {
		t.Errorf("Confirm() = %v, want false", got)
	}

	// Unknown returns default
	got, _ = p.Confirm("Unknown?", true)
	if got != true {
		t.Errorf("Confirm() = %v, want true (default)", got)
	}
}

func TestScriptedPrompter_PromptSelect(t *testing.T) {
	p := NewScriptedPrompter().
		OnPromptSelect("Choose", "second")

	// Scripted response
	got, _ := p.PromptSelect("Choose", []string{"first", "second"}, "first")
	if got != "second" {
		t.Errorf("PromptSelect() = %q, want %q", got, "second")
	}

	// Unknown returns default
	got, _ = p.PromptSelect("Unknown", []string{"a", "b"}, "a")
	if got != "a" {
		t.Errorf("PromptSelect() = %q, want %q", got, "a")
	}
}

// TestRequireConfirm_DestructiveCommandPromptsWithoutForce tests the Gherkin scenario:
// "destructive command prompts without --force"
// When user answers "no", operation is not performed (returns false), and exit code is 0.
func TestRequireConfirm_DestructiveCommandPromptsWithoutForce(t *testing.T) {
	tests := []struct {
		name       string
		userAnswer bool
		want       bool
	}{
		{
			name:       "user answers no - operation not performed",
			userAnswer: false,
			want:       false,
		},
		{
			name:       "user answers yes - operation performed",
			userAnswer: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompter := NewScriptedPrompter().
				OnConfirm("Delete organization org-1?", tt.userAnswer)

			got, err := RequireConfirm(prompter, "Delete organization org-1?", false)
			if err != nil {
				t.Fatalf("RequireConfirm() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("RequireConfirm() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRequireConfirm_DestructiveCommandDoesNotPromptWithForce tests the Gherkin scenario:
// "destructive command does not prompt with --force"
// When --force is set, no prompt is shown and operation proceeds.
func TestRequireConfirm_DestructiveCommandDoesNotPromptWithForce(t *testing.T) {
	// Create a prompter that would fail the test if called
	prompter := &failingPrompter{t: t}

	got, err := RequireConfirm(prompter, "Delete organization org-1?", true)
	if err != nil {
		t.Fatalf("RequireConfirm() error = %v", err)
	}
	if got != true {
		t.Errorf("RequireConfirm() = %v, want true (forced)", got)
	}
}

// failingPrompter fails the test if any prompt method is called.
type failingPrompter struct {
	t *testing.T
}

func (p *failingPrompter) PromptString(prompt, defaultValue string) (string, error) {
	p.t.Fatalf("PromptString called with %q when --force should bypass prompts", prompt)
	return "", nil
}

func (p *failingPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	p.t.Fatalf("Confirm called with %q when --force should bypass prompts", prompt)
	return false, nil
}

func (p *failingPrompter) PromptSelect(prompt string, options []string, defaultValue string) (string, error) {
	p.t.Fatalf("PromptSelect called with %q when --force should bypass prompts", prompt)
	return "", nil
}
