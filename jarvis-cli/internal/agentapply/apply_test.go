package agentapply

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// statuslineStub models ClaudeAgent.InstallStatusline (agent/claude.go:862-873):
// it consults confirm only when the script already exists on disk, and it does
// so unguarded, so a nil decision would panic on every upgrade.
type statuslineStub struct {
	scriptPresent bool
	err           error

	calls int
	wrote bool
}

func (s *statuslineStub) InstallStatusline(_ fs.FS, confirm func() bool) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	if s.scriptPresent && !confirm() {
		return nil
	}
	s.wrote = true
	return nil
}

// TestStatuslineDecisionFromState covers all four (Decided, Enabled)
// combinations crossed with script-present and script-absent. Only a recorded,
// enabled decision authorizes touching the statusline at all; once authorized,
// the manifest is the authority, so a script deleted from disk is drift and is
// reinstalled rather than treated as a revoked consent.
func TestStatuslineDecisionFromState(t *testing.T) {
	states := []struct {
		name    string
		st      state.StatuslineState
		managed bool
	}{
		{"never asked", state.StatuslineState{}, false},
		{"enabled without a recorded decision", state.StatuslineState{Enabled: true}, false},
		{"decided against", state.StatuslineState{Decided: true}, false},
		{"decided in favour", state.StatuslineState{Decided: true, Enabled: true}, true},
	}
	disks := []struct {
		name    string
		present bool
	}{
		{"script present on disk", true},
		{"script deleted from disk", false},
	}

	for _, tc := range states {
		for _, disk := range disks {
			t.Run(tc.name+", "+disk.name, func(t *testing.T) {
				decision := StatuslineDecisionFromState(tc.st)
				if decision.Confirm == nil {
					t.Fatal("StatuslineDecisionFromState() returned a nil Confirm; InstallStatusline calls it unguarded")
				}
				if decision.Install != tc.managed {
					t.Fatalf("StatuslineDecisionFromState().Install = %v, want %v", decision.Install, tc.managed)
				}

				stub := &statuslineStub{scriptPresent: disk.present}
				if err := ApplyStatusline(stub, nil, decision); err != nil {
					t.Fatalf("ApplyStatusline() error = %v", err)
				}

				wantCalls := 0
				if tc.managed {
					wantCalls = 1
				}
				if stub.calls != wantCalls {
					t.Fatalf("InstallStatusline called %d times, want %d", stub.calls, wantCalls)
				}
				if stub.wrote != tc.managed {
					t.Fatalf("statusline written = %v, want %v", stub.wrote, tc.managed)
				}
			})
		}
	}
}

func TestApplyStatuslineWrapsInstallFailure(t *testing.T) {
	stub := &statuslineStub{err: errors.New("boom")}

	err := ApplyStatusline(stub, nil, StatuslineDecisionFromState(state.StatuslineState{Decided: true, Enabled: true}))
	if err == nil || err.Error() != "install statusline: boom" {
		t.Fatalf("ApplyStatusline() error = %v, want \"install statusline: boom\"", err)
	}
}

func TestApplyStatuslineSkipsAgentsWithoutSupport(t *testing.T) {
	if err := ApplyStatusline(struct{}{}, nil, StatuslineDecisionFromState(state.StatuslineState{Decided: true, Enabled: true})); err != nil {
		t.Fatalf("ApplyStatusline() error = %v for an agent without statusline support", err)
	}
}
