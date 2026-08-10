package model

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestProjectBlockRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     ProjectBlockRequest
		project string
		wantErr bool
	}{
		{
			name:    "valid block request confirms canonical key exactly",
			project: "jarvis-dev",
			req: ProjectBlockRequest{
				Action:       ProjectBlockActionBlock,
				Reason:       "duplicate garbage project",
				Confirmation: "jarvis-dev",
				ExportMarker: "export-2026-07-05",
			},
		},
		{
			name:    "rejects display-name confirmation when canonical key is required",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "Jarvis Dev", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects confirmation with surrounding whitespace",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionBlock, Reason: "duplicate", Confirmation: " jarvis-dev ", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects missing reason",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionBlock, Confirmation: "jarvis-dev", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects wrong confirmation",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "other", ExportMarker: "export-1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate(tt.project)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestProjectBlockRequestValidateUsesQuarantineActionsWithoutLegacyExportMarker(t *testing.T) {
	tests := []struct {
		name    string
		req     ProjectBlockRequest
		wantErr bool
	}{
		{
			name: "accepts block without historical export marker",
			req: ProjectBlockRequest{
				Action:       ProjectBlockActionBlock,
				Reason:       "policy violation",
				Confirmation: "jarvis-dev",
			},
		},
		{
			name: "accepts unblock without historical export marker",
			req: ProjectBlockRequest{
				Action:       ProjectBlockActionUnblock,
				Reason:       "release approved",
				Confirmation: "jarvis-dev",
			},
		},
		{
			name: "rejects purge intent before a write can happen",
			req: ProjectBlockRequest{
				Action:       ProjectBlockActionPurgeIntent,
				Reason:       "not supported",
				Confirmation: "jarvis-dev",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate("jarvis-dev")
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestProjectBlockAckValidate(t *testing.T) {
	tests := []struct {
		name    string
		ack     ProjectBlockAck
		wantErr bool
	}{
		{name: "allows applied with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckApplied}},
		{name: "allows failed with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckFailed}},
		{name: "allows skipped with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckSkipped}},
		{name: "rejects forged status", ack: ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: "owned"}, wantErr: true},
		{name: "rejects missing command", ack: ProjectBlockAck{ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckApplied}, wantErr: true},
		{name: "rejects missing ack token", ack: ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", Status: ProjectBlockAckApplied}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ack.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestProjectBlockAckSubjectValidRequiresOnlySignedAccountSubject(t *testing.T) {
	tests := []struct {
		name    string
		subject ProjectBlockAckSubject
		want    bool
	}{
		{name: "allows authenticated account without daemon metadata", subject: ProjectBlockAckSubject{AuthSubject: "user-1"}, want: true},
		{name: "allows authenticated account with daemon metadata", subject: ProjectBlockAckSubject{AuthSubject: "user-1", DaemonID: "daemon-1", Client: "hive-daemon"}, want: true},
		{name: "rejects missing account subject even with daemon metadata", subject: ProjectBlockAckSubject{DaemonID: "daemon-1", Client: "hive-daemon"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.subject.Valid(); got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectBlockedErrorResponseCarriesCommand(t *testing.T) {
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	payload := ProjectBlockedErrorResponse{
		Error: "project is blocked",
		Command: ProjectBlockCommand{
			CommandID:  "cmd-1",
			AckToken:   "ack-token-1",
			Project:    "Jarvis Dev",
			ProjectKey: "jarvis-dev",
			Reason:     "duplicate",
			BlockedAt:  blockedAt,
		},
	}

	if payload.Command.ProjectKey != "jarvis-dev" {
		t.Fatalf("canonical key = %q", payload.Command.ProjectKey)
	}
	if payload.Command.BlockedAt.IsZero() {
		t.Fatal("blocked_at must be included in the 423 command payload")
	}
}

func TestProjectBlockCommandRedactedRemovesAckAuthority(t *testing.T) {
	cmd := ProjectBlockCommand{
		CommandID:  "cmd-1",
		AckToken:   "ack-token-secret",
		Project:    "Jarvis Dev",
		ProjectKey: "jarvis-dev",
		Reason:     "duplicate",
		BlockedAt:  time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC),
	}

	redacted := cmd.Redacted()

	if redacted.CommandID != "cmd-1" || redacted.ProjectKey != "jarvis-dev" {
		t.Fatalf("redacted command lost routing identity: %#v", redacted)
	}
	if redacted.AckToken != "" {
		t.Fatalf("redacted command leaked ack token %q", redacted.AckToken)
	}
	if redacted.Reason != "" {
		t.Fatalf("redacted command leaked reason %q", redacted.Reason)
	}
}

func TestProjectBlockCommandValidateRequiresMonotonicLifecycleFacts(t *testing.T) {
	tests := []struct {
		name    string
		command ProjectBlockCommand
		wantErr bool
	}{
		{
			name: "accepts a generation scoped block command",
			command: ProjectBlockCommand{
				CommandID:  "cmd-2",
				AckToken:   "ack-token-2",
				Project:    "Jarvis Dev",
				ProjectKey: "jarvis-dev",
				Action:     ProjectBlockActionBlock,
				Generation: 2,
			},
		},
		{
			name: "rejects an unsupported legacy action as a new command",
			command: ProjectBlockCommand{
				CommandID:  "cmd-2",
				AckToken:   "ack-token-2",
				ProjectKey: "jarvis-dev",
				Action:     ProjectBlockActionQuarantine,
				Generation: 2,
			},
			wantErr: true,
		},
		{
			name: "rejects a command without a generation",
			command: ProjectBlockCommand{
				CommandID:  "cmd-2",
				AckToken:   "ack-token-2",
				ProjectKey: "jarvis-dev",
				Action:     ProjectBlockActionUnblock,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.command.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestQuarantineCursorBindsPaginationToProjectAndGeneration(t *testing.T) {
	cursor := QuarantineCursor{
		ProjectKey: "org-repo",
		Generation: 7,
		Username:   "Ada",
		CursorID:   "opaque-user-ordering-key",
	}

	decoded, err := DecodeQuarantineCursor(cursor.Encode(), "org-repo", 7)
	if err != nil {
		t.Fatalf("DecodeQuarantineCursor() error = %v", err)
	}
	if decoded != cursor {
		t.Fatalf("DecodeQuarantineCursor() = %#v, want %#v", decoded, cursor)
	}

	for _, target := range []struct {
		project    string
		generation int64
	}{
		{project: "other-repo", generation: 7},
		{project: "org-repo", generation: 8},
	} {
		if _, err := DecodeQuarantineCursor(cursor.Encode(), target.project, target.generation); err == nil {
			t.Fatalf("DecodeQuarantineCursor() accepted cursor for %q generation %d", target.project, target.generation)
		}
	}
}

func TestQuarantineCursorDoesNotEmbedAccountID(t *testing.T) {
	cursor := QuarantineCursor{
		ProjectKey: "org-repo",
		Generation: 7,
		Username:   "Ada",
		CursorID:   "opaque-user-ordering-key",
	}

	encoded, err := base64.RawURLEncoding.DecodeString(cursor.Encode())
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if strings.Contains(string(encoded), "user_id") {
		t.Fatalf("cursor unexpectedly serializes user_id: %s", encoded)
	}
}
