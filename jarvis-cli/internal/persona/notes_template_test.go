package persona

import "testing"

func TestValidateNotesTemplate(t *testing.T) {
	tests := []struct {
		name    string
		notes   string
		wantErr bool
	}{
		{
			name: "canonical template",
			notes: `# Custom Mentor

## Core Principle

Help first and keep the answer focused.

## Behavior

1. Explain the why before code when complexity matters.

## When Asking Questions

Ask one question and stop.
`,
		},
		{
			name: "canonical template allows extra sections",
			notes: `# Custom Mentor

## Core Principle

Help first and keep the answer focused.

## Behavior

1. Explain the why before code when complexity matters.

## Language Rules

Spanish input uses voseo naturally.

## When Asking Questions

Ask one question and stop.
`,
		},
		{
			name: "invalid missing required section",
			notes: `# Custom Mentor

## Core Principle

Help first and keep the answer focused.

## Tone

Direct and warm.
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotesTemplate(tt.notes)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateNotesTemplate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateNotesTemplate() unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePreset_NotesEditorialContract(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid schema and canonical notes",
			yaml: `name: custom-mentor
display_name: "Custom Mentor"
description: "User-defined preset"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hey"]
  confirmations: ["Done"]
notes: |
  # Custom Mentor

  ## Core Principle

  Help first.

  ## Behavior

  Explain the why.

  ## When Asking Questions

  Ask one question and stop.
`,
		},
		{
			name: "invalid notes template",
			yaml: `name: custom-mentor
display_name: "Custom Mentor"
description: "User-defined preset"
tone:
  formality: informal
  directness: high
  humor: wholesome
  language: en-us
communication_style:
  verbosity: moderate
  show_alternatives: true
  challenge_assumptions: true
characteristic_phrases:
  greetings: ["Hey"]
  confirmations: ["Done"]
notes: |
  # Custom Mentor

  ## Core Principle

  Help first.
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePreset([]byte(tt.yaml))
			if tt.wantErr && err == nil {
				t.Fatalf("ValidatePreset() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidatePreset() unexpected error: %v", err)
			}
		})
	}
}
