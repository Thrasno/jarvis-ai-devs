package sync

import (
	"errors"
	"testing"
)

func TestMaskPassword(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string returns empty", input: "", want: ""},
		{name: "non-empty returns sentinel", input: "anything", want: MaskedSecret},
		{name: "already masked sentinel returns sentinel", input: "********", want: MaskedSecret},
		{name: "single char returns sentinel", input: "x", want: MaskedSecret},
		{name: "long password returns sentinel", input: "very-long-password-12345", want: MaskedSecret},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskPassword(tc.input)
			if got != tc.want {
				t.Errorf("maskPassword(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolvePassword(t *testing.T) {
	storedPass := "storedSecret"
	storedCfg := &Config{Password: storedPass}
	emptyStoredCfg := &Config{Password: ""}

	cases := []struct {
		name        string
		incoming    string
		stored      *Config
		want        string
		wantErr     error
	}{
		{
			name:     "new password (not sentinel) returns incoming",
			incoming: "newpass",
			stored:   storedCfg,
			want:     "newpass",
		},
		{
			name:     "new password ignores stored",
			incoming: "differentpass",
			stored:   nil,
			want:     "differentpass",
		},
		{
			name:     "sentinel with stored config returns stored password",
			incoming: MaskedSecret,
			stored:   storedCfg,
			want:     storedPass,
		},
		{
			name:    "sentinel with nil stored config returns ErrNoStoredSecret",
			incoming: MaskedSecret,
			stored:  nil,
			wantErr: ErrNoStoredSecret,
		},
		{
			name:    "sentinel with empty stored password returns ErrNoStoredSecret",
			incoming: MaskedSecret,
			stored:  emptyStoredCfg,
			wantErr: ErrNoStoredSecret,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePassword(tc.incoming, tc.stored)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("resolvePassword() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePassword() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolvePassword() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMaskedSecretConstantValue(t *testing.T) {
	// The exact value is load-bearing — the TUI uses this sentinel for round-trips.
	if MaskedSecret != "********" {
		t.Errorf("MaskedSecret = %q, want exactly %q", MaskedSecret, "********")
	}
}
