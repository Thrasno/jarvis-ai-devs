package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestNativeMCPSnapshotBlocksUnownedSameNameAfterGetOnly(t *testing.T) {
	fake := newNativeMCPInventoryFake(map[string]string{
		"jarvis-hive": `{"token":"foreign-secret"}`,
	})

	_, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{nativeMCPDefinition("jarvis-hive", "expected-secret")})
	if err == nil || !strings.Contains(err.Error(), "ownership is not proven") {
		t.Fatalf("Snapshot() error = %v, want ownership rejection", err)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "jarvis-hive"}) {
		t.Fatalf("calls = %#v, want one get only", fake.calls)
	}
}

func TestNativeMCPSnapshotClassifiesAbsentDesiredIdentityAsCreatable(t *testing.T) {
	fake := newNativeMCPInventoryFake(nil)
	journal, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{nativeMCPDefinition("jarvis-hive", "expected-secret")})
	if err != nil {
		t.Fatalf("Snapshot() error = %v, want absent identity to be creatable", err)
	}
	if len(journal.Managed) != 0 {
		t.Fatalf("managed = %#v, want no managed identities", journal.Managed)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "jarvis-hive"}) {
		t.Fatalf("calls = %#v, want one get only", fake.calls)
	}
}

func TestNativeMCPSnapshotCreatesOnlyForExplicitClaudeMCPAbsentResponse(t *testing.T) {
	const identity = "jarvis-hive"

	tests := []struct {
		name          string
		result        claudeCommandResult
		wantCreatable bool
	}{
		{
			name:   "executable absent",
			result: claudeCommandResult{Err: os.ErrNotExist},
		},
		{
			name:   "permission denied",
			result: claudeCommandResult{Err: os.ErrPermission},
		},
		{
			name:   "timeout",
			result: claudeCommandResult{Err: context.DeadlineExceeded, Started: true},
		},
		{
			name:   "malformed output",
			result: claudeCommandResult{Output: "{", Err: errors.New("exit status 1"), Started: true},
		},
		{
			name:   "unrelated not-found text",
			result: claudeCommandResult{Output: "configuration file not found", Err: errors.New("exit status 1"), Started: true},
		},
		{
			name:          "explicit Claude MCP-absent response",
			result:        claudeCommandResult{Output: "Error: MCP server 'jarvis-hive' not found", Err: errors.New("exit status 1"), Started: true},
			wantCreatable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			manager := NativeMCPManager{run: func(_ string, _ ...string) claudeCommandResult {
				calls++
				return tt.result
			}}

			journal, err := manager.Snapshot([]NativeMCPDefinition{nativeMCPDefinition(identity, "expected-secret")})
			if tt.wantCreatable {
				if err != nil {
					t.Fatalf("Snapshot() error = %v, want creatable identity", err)
				}
				if len(journal.Managed) != 0 {
					t.Fatalf("managed = %#v, want no managed identities", journal.Managed)
				}
			} else if err == nil {
				t.Fatal("Snapshot() error = nil, want fail-closed rejection")
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want one get", calls)
			}
		})
	}
}

func TestNativeMCPSnapshotRejectsSelfConsistentForgedManifestWithoutTrustedFingerprint(t *testing.T) {
	trusted := nativeMCPDefinition("jarvis-hive", "trusted-secret")
	forged := nativeMCPOutput("jarvis-hive", "forged-secret")
	fake := newNativeMCPInventoryFake(map[string]string{"jarvis-hive": forged})

	_, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{trusted})
	if err == nil || !strings.Contains(err.Error(), "ownership is not proven") {
		t.Fatalf("Snapshot() error = %v, want independently trusted fingerprint rejection", err)
	}
}

func TestNativeMCPSnapshotStoresOnlySecretSafeInventoryEvidence(t *testing.T) {
	definition := nativeMCPDefinition("jarvis-hive", "super-secret")
	fake := newNativeMCPInventoryFake(map[string]string{"jarvis-hive": nativeMCPOutput("jarvis-hive", "super-secret")})

	journal, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{definition})
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if journal.Phase != NativeMCPSnapshotted || len(journal.Managed) != 1 {
		t.Fatalf("journal = %#v, want one snapshotted managed identity", journal)
	}
	if diagnostics := journal.Diagnostics(); strings.Contains(diagnostics, "super-secret") {
		t.Fatalf("Diagnostics() leaked secret: %q", diagnostics)
	}
}

type nativeMCPInventoryFake struct {
	servers map[string]string
	calls   [][]string
}

func newNativeMCPInventoryFake(servers map[string]string) *nativeMCPInventoryFake {
	copyServers := make(map[string]string, len(servers))
	for identity, output := range servers {
		copyServers[identity] = output
	}
	return &nativeMCPInventoryFake{servers: copyServers}
}

func (f *nativeMCPInventoryFake) run(_ string, args ...string) claudeCommandResult {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) != 3 || args[0] != "mcp" || args[1] != "get" {
		return claudeCommandResult{Err: fmt.Errorf("unexpected command: %v", args)}
	}
	output, exists := f.servers[args[2]]
	if !exists {
		return claudeCommandResult{Output: "Error: MCP server '" + args[2] + "' not found", Err: errors.New("exit status 1"), Started: true}
	}
	return claudeCommandResult{Output: output, Started: true}
}

func nativeMCPDefinition(identity, secret string) NativeMCPDefinition {
	configuration := fmt.Sprintf(`{"identity":%q,"token":%q}`, identity, secret)
	return NativeMCPDefinition{
		Identity:            identity,
		Scope:               "user",
		SchemaVersion:       "v1",
		AddArgs:             []string{"mcp", "add", "--scope", "user", identity, secret},
		ExpectedFingerprint: nativeMCPFingerprint(configuration),
	}
}

func nativeMCPOutput(identity, secret string) string {
	configuration := fmt.Sprintf(`{"identity":%q,"token":%q}`, identity, secret)
	return fmt.Sprintf(`{"identity":%q,"token":%q,"jarvis_provenance":{"schema_version":"v1","managed_identity":%q,"scope":"user","configuration_sha256":%q}}`, identity, secret, identity, nativeMCPFingerprint(configuration))
}

func sameNativeMCPCall(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
