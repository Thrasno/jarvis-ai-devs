package model

import (
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
			name:    "valid quarantine request confirms canonical key exactly",
			project: "jarvis-dev",
			req: ProjectBlockRequest{
				Action:       ProjectBlockActionQuarantine,
				Reason:       "duplicate garbage project",
				Confirmation: "jarvis-dev",
				ExportMarker: "export-2026-07-05",
			},
		},
		{
			name:    "rejects display-name confirmation when canonical key is required",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionQuarantine, Reason: "duplicate", Confirmation: "Jarvis Dev", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects confirmation with surrounding whitespace",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionQuarantine, Reason: "duplicate", Confirmation: " jarvis-dev ", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects missing reason",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionQuarantine, Confirmation: "jarvis-dev", ExportMarker: "export-1"},
			wantErr: true,
		},
		{
			name:    "rejects wrong confirmation",
			project: "jarvis-dev",
			req:     ProjectBlockRequest{Action: ProjectBlockActionQuarantine, Reason: "duplicate", Confirmation: "other", ExportMarker: "export-1"},
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

func TestProjectBlockAckValidate(t *testing.T) {
	tests := []struct {
		name    string
		ack     ProjectBlockAck
		wantErr bool
	}{
		{name: "allows applied with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckApplied}},
		{name: "allows failed with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckFailed}},
		{name: "allows skipped with ack token", ack: ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckSkipped}},
		{name: "rejects forged status", ack: ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: "owned"}, wantErr: true},
		{name: "rejects missing command", ack: ProjectBlockAck{CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: ProjectBlockAckApplied}, wantErr: true},
		{name: "rejects missing ack token", ack: ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", Status: ProjectBlockAckApplied}, wantErr: true},
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
			CommandID:           "cmd-1",
			AckToken:            "ack-token-1",
			Project:             "Jarvis Dev",
			CanonicalProjectKey: "jarvis-dev",
			Reason:              "duplicate",
			BlockedAt:           blockedAt,
		},
	}

	if payload.Command.CanonicalProjectKey != "jarvis-dev" {
		t.Fatalf("canonical key = %q", payload.Command.CanonicalProjectKey)
	}
	if payload.Command.BlockedAt.IsZero() {
		t.Fatal("blocked_at must be included in the 423 command payload")
	}
}

func TestProjectBlockCommandRedactedRemovesAckAuthority(t *testing.T) {
	cmd := ProjectBlockCommand{
		CommandID:           "cmd-1",
		AckToken:            "ack-token-secret",
		Project:             "Jarvis Dev",
		CanonicalProjectKey: "jarvis-dev",
		Reason:              "duplicate",
		BlockedAt:           time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC),
	}

	redacted := cmd.Redacted()

	if redacted.CommandID != "cmd-1" || redacted.CanonicalProjectKey != "jarvis-dev" {
		t.Fatalf("redacted command lost routing identity: %#v", redacted)
	}
	if redacted.AckToken != "" {
		t.Fatalf("redacted command leaked ack token %q", redacted.AckToken)
	}
	if redacted.Reason != "" {
		t.Fatalf("redacted command leaked reason %q", redacted.Reason)
	}
}
