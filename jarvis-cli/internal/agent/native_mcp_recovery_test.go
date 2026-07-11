package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNativeMCPSnapshotBlocksUnownedSameNameAfterGetOnly(t *testing.T) {
	fake := newNativeMCPInventoryFake(map[string]string{
		"jarvis-hive": `{"token":"foreign-secret"}`,
	})

	_, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{nativeMCPDefinition("jarvis-hive", "expected-secret")})
	if err == nil || !strings.Contains(err.Error(), "ownership is not proven") {
		t.Fatalf("Snapshot() error = %v, want ownership rejection", err)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "--scope", "user", "jarvis-hive"}) {
		t.Fatalf("calls = %#v, want one user-scope get only", fake.calls)
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
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "--scope", "user", "jarvis-hive"}) {
		t.Fatalf("calls = %#v, want one user-scope get only", fake.calls)
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

func TestRunNativeMCPInventoryCommandReturnsDeadlineExceededAfterStartingProcess(t *testing.T) {
	previousTimeout := nativeMCPInventoryCommandTimeout
	nativeMCPInventoryCommandTimeout = 100 * time.Millisecond
	t.Cleanup(func() { nativeMCPInventoryCommandTimeout = previousTimeout })
	t.Setenv("GO_WANT_NATIVE_MCP_INVENTORY_TIMEOUT_HELPER", "1")

	result := runNativeMCPInventoryCommand(os.Args[0], "-test.run=TestNativeMCPInventoryCommandTimeoutHelper", "--")

	if !result.Started {
		t.Fatal("Started = false, want process-start evidence")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("Err = %v, want context deadline exceeded", result.Err)
	}
}

func TestNativeMCPInventoryCommandTimeoutHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NATIVE_MCP_INVENTORY_TIMEOUT_HELPER") != "1" {
		return
	}
	time.Sleep(2 * time.Second)
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

func TestNativeMCPSnapshotInspectsOnlyUserScopedSameName(t *testing.T) {
	desired := nativeMCPDefinition("jarvis-hive", "expected-secret")
	fake := newNativeMCPScopeFake(map[string]map[string]string{
		"local":   {desired.Identity: nativeMCPOutput(desired.Identity, "local-secret")},
		"project": {desired.Identity: nativeMCPOutput(desired.Identity, "project-secret")},
		"user":    {},
	})

	journal, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{desired})

	if err != nil {
		t.Fatalf("Snapshot() error = %v, want absent user-scoped identity to be creatable", err)
	}
	if len(journal.Managed) != 0 {
		t.Fatalf("managed = %#v, want local/project variants ignored", journal.Managed)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "--scope", "user", desired.Identity}) {
		t.Fatalf("calls = %#v, want one explicit user-scope get", fake.calls)
	}
	if fake.servers["local"][desired.Identity] == "" || fake.servers["project"][desired.Identity] == "" {
		t.Fatalf("same-name local/project variants were changed: %#v", fake.servers)
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
	if len(args) != 5 || args[0] != "mcp" || args[1] != "get" || args[2] != "--scope" || args[3] != nativeMCPUserScope {
		return claudeCommandResult{Err: fmt.Errorf("unexpected command: %v", args)}
	}
	output, exists := f.servers[args[4]]
	if !exists {
		return claudeCommandResult{Output: "Error: MCP server '" + args[4] + "' not found", Err: errors.New("exit status 1"), Started: true}
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

func containsNativeMCPCall(calls [][]string, want []string) bool {
	for _, call := range calls {
		if sameNativeMCPCall(call, want) {
			return true
		}
	}
	return false
}

func TestNativeMCPReplaceSkipsEmptyInputWithoutCommands(t *testing.T) {
	fake := newNativeMCPScopeFake(nil)

	result, err := (NativeMCPManager{run: fake.run}).Replace(nil)

	if err != nil || result.Phase != NativeMCPSkipped || len(fake.calls) != 0 {
		t.Fatalf("Replace(nil) = (%#v, %v), calls=%#v; want skip with no commands", result, err, fake.calls)
	}
}

func TestNativeMCPReplaceConvergesNameGloballyAndPreservesNonTargets(t *testing.T) {
	desired := nativeMCPDefinition("jarvis-hive", "desired-secret")
	fake := newNativeMCPScopeFake(map[string]map[string]string{
		"local":   {desired.Identity: `{"token":"manual-local"}`},
		"project": {desired.Identity: nativeMCPOutput(desired.Identity, "managed-project")},
		"user":    {desired.Identity: `{"token":"manual-user"}`, "unrelated": `{"token":"leave-me"}`},
	})
	fake.addOutput = nativeMCPOutput(desired.Identity, "desired-secret")

	result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})

	if err != nil || result.Phase != NativeMCPVerified || result.TargetName != desired.Identity || result.FixedLocation != "claude --scope user" {
		t.Fatalf("Replace() = (%#v, %v), want verified user result", result, err)
	}
	if got := fake.servers["local"][desired.Identity]; got != `{"token":"manual-local"}` {
		t.Fatalf("local desired-name variant = %q, want untouched", got)
	}
	if got := fake.servers["project"][desired.Identity]; got != nativeMCPOutput(desired.Identity, "managed-project") {
		t.Fatalf("project desired-name variant = %q, want untouched", got)
	}
	if got := fake.servers["user"][desired.Identity]; got != fake.addOutput {
		t.Fatalf("user desired server = %q, want desired output", got)
	}
	if got := fake.servers["user"]["unrelated"]; got != `{"token":"leave-me"}` {
		t.Fatalf("non-target = %q, want untouched", got)
	}
	for _, call := range fake.calls {
		if call[len(call)-1] == "unrelated" || containsString(call, "local") || containsString(call, "project") {
			t.Fatalf("calls = %#v, non-target received a command", fake.calls)
		}
	}
}

func TestNativeMCPReplaceRejectsNonUserDefinitionBeforeCommands(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NativeMCPDefinition)
	}{
		{
			name: "non-user scope",
			mutate: func(desired *NativeMCPDefinition) {
				desired.Scope = "project"
				desired.AddArgs = []string{"mcp", "add", "--scope", "project", desired.Identity, "desired-secret"}
			},
		},
		{
			name: "add command scope does not match fixed user scope",
			mutate: func(desired *NativeMCPDefinition) {
				desired.AddArgs[3] = "local"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := nativeMCPDefinition("jarvis-hive", "desired-secret")
			tt.mutate(&desired)
			fake := newNativeMCPScopeFake(nil)

			result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})

			if err == nil || result.Phase != NativeMCPInspected || result.ErrorCategory != "definition" || result.ErrorCode != "invalid" {
				t.Fatalf("Replace() = (%#v, %v), want invalid user-scope definition", result, err)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("calls = %#v, want no native commands", fake.calls)
			}
		})
	}
}

func TestNativeMCPReplaceStopsAtEachFailureWithoutSecrets(t *testing.T) {
	desired := nativeMCPDefinition("jarvis-hive", "desired-secret")
	for _, phase := range []NativeMCPPhase{NativeMCPInspected, NativeMCPRemoved, NativeMCPAdded, NativeMCPVerifying} {
		t.Run(string(phase), func(t *testing.T) {
			fake := newNativeMCPScopeFake(map[string]map[string]string{"user": {desired.Identity: `{"token":"manual-secret"}`}})
			fake.addOutput = nativeMCPOutput(desired.Identity, "desired-secret")
			fake.failPhase = phase

			result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})

			if err == nil || result.Phase != phase || result.TargetName != desired.Identity || result.ErrorCategory == "" || result.ErrorCode == "" || result.Guidance == "" {
				t.Fatalf("Replace() = (%#v, %v), want actionable fail-stop result", result, err)
			}
			if strings.Contains(result.Diagnostics(), "manual-secret") || strings.Contains(result.Diagnostics(), "desired-secret") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("failure leaked secret: result=%#v error=%v", result, err)
			}
			if fake.failedAt != len(fake.calls)-1 {
				t.Fatalf("calls continued after %s failure: %#v", phase, fake.calls)
			}
		})
	}
}

func TestNativeMCPReplaceRerunConvergesAfterPartialAddFailure(t *testing.T) {
	desired := nativeMCPDefinition("jarvis-hive", "desired-secret")
	fake := newNativeMCPScopeFake(map[string]map[string]string{"project": {desired.Identity: `{"token":"manual"}`}})
	fake.addOutput = nativeMCPOutput(desired.Identity, "desired-secret")
	fake.failPhase = NativeMCPAdded
	fake.mutateBeforeFailure = true

	result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})
	if err == nil || result.Phase != NativeMCPAdded || fake.servers["user"][desired.Identity] == "" {
		t.Fatalf("first Replace() = (%#v, %v), want partial add failure", result, err)
	}
	fake.failPhase = ""
	result, err = (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})
	if err != nil || result.Phase != NativeMCPVerified || len(fake.servers["user"]) != 1 || fake.servers["user"][desired.Identity] == "" {
		t.Fatalf("rerun Replace() = (%#v, %v), user=%#v; want convergence", result, err, fake.servers["user"])
	}
}

type nativeMCPScopeFake struct {
	servers             map[string]map[string]string
	addOutput           string
	failPhase           NativeMCPPhase
	mutateBeforeFailure bool
	added               bool
	calls               [][]string
	failedAt            int
}

func newNativeMCPScopeFake(servers map[string]map[string]string) *nativeMCPScopeFake {
	copyScopes := make(map[string]map[string]string)
	for _, scope := range []string{"local", "project", nativeMCPUserScope} {
		copyScopes[scope] = make(map[string]string)
		for name, output := range servers[scope] {
			copyScopes[scope][name] = output
		}
	}
	return &nativeMCPScopeFake{servers: copyScopes, failedAt: -1}
}

func (f *nativeMCPScopeFake) run(_ string, args ...string) claudeCommandResult {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) < 5 || args[0] != "mcp" || args[2] != "--scope" {
		return claudeCommandResult{Err: errors.New("unexpected command"), Started: true}
	}
	phase := NativeMCPPhase(map[string]NativeMCPPhase{"get": NativeMCPInspected, "remove": NativeMCPRemoved, "add": NativeMCPAdded}[args[1]])
	if args[1] == "get" && f.failPhase == NativeMCPVerifying && f.hasAddedDesired() {
		phase = NativeMCPVerifying
	}
	if f.failPhase == phase {
		f.failedAt = len(f.calls) - 1
		if phase == NativeMCPAdded && f.mutateBeforeFailure {
			f.servers[args[3]][args[4]] = f.addOutput
			f.added = true
		}
		return claudeCommandResult{Output: "token=manual-secret", Err: errors.New("exit status 17: desired-secret"), Started: true}
	}
	scope, name := args[3], args[4]
	switch args[1] {
	case "get":
		output, exists := f.servers[scope][name]
		if !exists {
			return claudeCommandResult{Output: "Error: MCP server '" + name + "' not found", Err: errors.New("exit status 1"), Started: true}
		}
		return claudeCommandResult{Output: output, Started: true}
	case "remove":
		delete(f.servers[scope], name)
		return claudeCommandResult{Started: true}
	case "add":
		f.servers[scope][name] = f.addOutput
		f.added = true
		return claudeCommandResult{Started: true}
	default:
		return claudeCommandResult{Err: errors.New("unexpected command"), Started: true}
	}
}

func (f *nativeMCPScopeFake) hasAddedDesired() bool {
	return f.added
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
