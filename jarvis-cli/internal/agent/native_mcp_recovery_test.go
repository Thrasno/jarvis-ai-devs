package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeMCPSnapshotBlocksUnownedSameNameAfterGetOnly(t *testing.T) {
	fake := newNativeMCPInventoryFake(map[string]string{
		"jarvis-hive": "jarvis-hive:\n  Scope: Project config\n  Token: foreign-secret",
	})

	_, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{nativeMCPDefinition("jarvis-hive", "expected-secret")})
	if err == nil || !strings.Contains(err.Error(), "wrong-scope/project-config") {
		t.Fatalf("Snapshot() error = %v, want sanitized scope rejection", err)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", "jarvis-hive"}) {
		t.Fatalf("calls = %#v, want one unscoped get only", fake.calls)
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
		t.Fatalf("calls = %#v, want one unscoped get only", fake.calls)
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

func TestNativeMCPSnapshotStoresOnlySecretSafeInventoryEvidence(t *testing.T) {
	definition := nativeMCPDefinition("jarvis-hive", "super-secret")
	fake := newNativeMCPInventoryFake(map[string]string{"jarvis-hive": teammateClaudeMCPGetOutput("jarvis-hive")})

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

func TestNativeMCPSnapshotFailsClosedWhenGetIsShadowedByNonUserScope(t *testing.T) {
	desired := nativeMCPDefinition("jarvis-hive", "expected-secret")
	fake := newNativeMCPScopeFake(map[string]map[string]string{
		"local":   {desired.Identity: "Scope: Local config\nToken: local-secret"},
		"project": {desired.Identity: "Scope: Project config\nToken: project-secret"},
		"user":    {},
	})

	_, err := (NativeMCPManager{run: fake.run}).Snapshot([]NativeMCPDefinition{desired})

	if err == nil || !strings.Contains(err.Error(), "wrong-scope/local-config") {
		t.Fatalf("Snapshot() error = %v, want fail-closed shadowing diagnostic", err)
	}
	if len(fake.calls) != 1 || !sameNativeMCPCall(fake.calls[0], []string{"mcp", "get", desired.Identity}) {
		t.Fatalf("calls = %#v, want one unscoped get", fake.calls)
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

func TestClaudeUserMCPDefinitionsReturnsCanonicalDefinitions(t *testing.T) {
	directory := t.TempDir()
	daemonPath := filepath.Join(directory, "hive-daemon.exe")
	if err := os.WriteFile(daemonPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	hive, context7, err := ClaudeUserMCPDefinitions(" \t" + daemonPath + "\n")
	if err != nil {
		t.Fatalf("ClaudeUserMCPDefinitions() error = %v", err)
	}

	assertNativeMCPDefinition(t, hive, NativeMCPDefinition{
		Identity:            "hive",
		Scope:               nativeMCPUserScope,
		SchemaVersion:       "v1",
		AddArgs:             []string{"mcp", "add", "--transport", "stdio", "--scope", "user", "hive", "--", daemonPath},
		ExpectedFingerprint: nativeMCPFingerprint(`{"type":"stdio","command":` + strconv.Quote(daemonPath) + `,"args":[]}`),
	})
	assertNativeMCPDefinition(t, context7, NativeMCPDefinition{
		Identity:            "context7",
		Scope:               nativeMCPUserScope,
		SchemaVersion:       "v1",
		AddArgs:             []string{"mcp", "add", "--transport", "http", "--scope", "user", "context7", "https://mcp.context7.com/mcp"},
		ExpectedFingerprint: nativeMCPFingerprint(`{"type":"http","url":"https://mcp.context7.com/mcp"}`),
	})
}

func TestClaudeUserMCPDefinitionsRejectsInvalidDaemonPaths(t *testing.T) {
	directory := t.TempDir()
	nonExecutablePath := filepath.Join(directory, "not-executable")
	if err := os.WriteFile(nonExecutablePath, []byte("daemon"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	for _, path := range []string{"", directory, filepath.Join(directory, "missing"), nonExecutablePath} {
		t.Run(path, func(t *testing.T) {
			_, _, err := ClaudeUserMCPDefinitions(path)
			if err == nil {
				t.Fatal("ClaudeUserMCPDefinitions() error = nil, want rejection")
			}
			if path != "" && strings.Contains(err.Error(), path) {
				t.Fatalf("error %q exposes daemon path %q", err, path)
			}
		})
	}
}

func TestClaudeUserMCPDefinitionsBuildsWizardRequestWithoutTUIPolicy(t *testing.T) {
	directory := t.TempDir()
	daemonPath := filepath.Join(directory, "hive-daemon.exe")
	if err := os.WriteFile(daemonPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	hive, context7, err := ClaudeUserMCPDefinitions(daemonPath)
	if err != nil {
		t.Fatalf("ClaudeUserMCPDefinitions() error = %v", err)
	}
	request, err := BuildWizardReconcileRequest(WizardReconcileInput{
		SelectedAgents: []string{"claude"},
		Root:           directory,
		EvidencePath:   filepath.Join(directory, "recovery.json"),
		ClaudeHive:     hive,
		ClaudeContext7: context7,
	})
	if err != nil {
		t.Fatalf("BuildWizardReconcileRequest() error = %v", err)
	}
	if len(request.DesiredMCPs) != 2 {
		t.Fatalf("DesiredMCPs = %#v, want both canonical definitions", request.DesiredMCPs)
	}
	assertNativeMCPDefinition(t, request.DesiredMCPs[0], hive)
	assertNativeMCPDefinition(t, request.DesiredMCPs[1], context7)
}

func assertNativeMCPDefinition(t *testing.T, got, want NativeMCPDefinition) {
	t.Helper()
	if got.Identity != want.Identity || got.Scope != want.Scope || got.SchemaVersion != want.SchemaVersion ||
		got.ExpectedFingerprint != want.ExpectedFingerprint || !sameNativeMCPCall(got.AddArgs, want.AddArgs) {
		t.Fatalf("definition = %#v, want %#v", got, want)
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
		"local":   {"local-only": `{"token":"manual-local"}`},
		"project": {"project-only": `{"token":"managed-project"}`},
		"user":    {desired.Identity: teammateClaudeMCPGetOutput(desired.Identity), "unrelated": `{"token":"leave-me"}`},
	})
	fake.addOutput = teammateClaudeMCPGetOutput(desired.Identity)

	result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})

	if err != nil || result.Phase != NativeMCPVerified || result.TargetName != desired.Identity || result.FixedLocation != "claude --scope user" {
		t.Fatalf("Replace() = (%#v, %v), want verified user result", result, err)
	}
	if got := fake.servers["local"]["local-only"]; got != `{"token":"manual-local"}` {
		t.Fatalf("local non-target = %q, want untouched", got)
	}
	if got := fake.servers["project"]["project-only"]; got != `{"token":"managed-project"}` {
		t.Fatalf("project non-target = %q, want untouched", got)
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

func TestNativeMCPReplaceCommandContractAndScopeParsing(t *testing.T) {
	tests := []struct {
		name       string
		getOutput  string
		wantCode   string
		wantRemove bool
	}{
		{name: "teammate Windows output", getOutput: teammateClaudeMCPGetOutput("hive"), wantRemove: true},
		{name: "casing and spacing", getOutput: "hive:\r\n\t sCoPe  :   uSeR CoNfIg  \r\n", wantRemove: true},
		{name: "project shadow", getOutput: "Scope: Project config", wantCode: "project-config"},
		{name: "local shadow", getOutput: " scope : local CONFIG ", wantCode: "local-config"},
		{name: "missing scope", getOutput: "Status: Connected", wantCode: "missing"},
		{name: "malformed scope", getOutput: "Scope user config", wantCode: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := nativeMCPDefinition("hive", "desired-secret")
			fake := newNativeMCPScopeFake(map[string]map[string]string{"user": {"hive": tt.getOutput}})
			fake.addOutput = teammateClaudeMCPGetOutput("hive")

			result, err := (NativeMCPManager{run: fake.run}).Replace([]NativeMCPDefinition{desired})
			if tt.wantCode == "" {
				if err != nil || result.Phase != NativeMCPVerified {
					t.Fatalf("Replace() = (%#v, %v), want verified", result, err)
				}
			} else if err == nil || result.ErrorCategory != "wrong-scope" || result.ErrorCode != tt.wantCode {
				t.Fatalf("Replace() = (%#v, %v), want wrong-scope/%s", result, err, tt.wantCode)
			}
			for _, call := range fake.calls {
				if len(call) >= 2 && call[1] == "get" && (!sameNativeMCPCall(call, []string{"mcp", "get", "hive"}) || containsString(call, "--scope")) {
					t.Fatalf("get argv = %#v, want exact unscoped get", call)
				}
				if len(call) >= 2 && (call[1] == "add" || call[1] == "remove") && !containsScopeUser(call) {
					t.Fatalf("mutation argv = %#v, want --scope user", call)
				}
			}
			if !tt.wantRemove && containsNativeMCPCall(fake.calls, []string{"mcp", "remove", "--scope", "user", "hive"}) {
				t.Fatalf("calls = %#v, wrong scope authorized removal", fake.calls)
			}
		})
	}
}

func TestNativeMCPReplaceUsesUnscopedGetAndUserScopedMutationsForHiveAndContext7(t *testing.T) {
	definitions := []NativeMCPDefinition{
		nativeMCPDefinition("hive", "hive-secret"),
		nativeMCPDefinition("context7", "context7-secret"),
	}
	fake := newNativeMCPScopeFake(nil)
	fake.addOutput = "Scope: User config"

	result, err := (NativeMCPManager{run: fake.run}).Replace(definitions)
	if err != nil || result.Phase != NativeMCPVerified || result.TargetName != "context7" {
		t.Fatalf("Replace() = (%#v, %v), want both managed names verified", result, err)
	}
	for _, identity := range []string{"hive", "context7"} {
		if !containsNativeMCPCall(fake.calls, []string{"mcp", "get", identity}) {
			t.Fatalf("calls = %#v, want exact unscoped get for %s", fake.calls, identity)
		}
		if !containsNativeMCPCall(fake.calls, []string{"mcp", "add", "--scope", "user", identity, identity + "-secret"}) {
			t.Fatalf("calls = %#v, want user-scoped add for %s", fake.calls, identity)
		}
	}
	for _, call := range fake.calls {
		if len(call) >= 2 && call[1] == "get" && containsString(call, "--scope") {
			t.Fatalf("get argv contains forbidden --scope: %#v", call)
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
			fake := newNativeMCPScopeFake(map[string]map[string]string{"user": {desired.Identity: teammateClaudeMCPGetOutput(desired.Identity)}})
			fake.addOutput = teammateClaudeMCPGetOutput(desired.Identity)
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
	fake := newNativeMCPScopeFake(nil)
	fake.addOutput = teammateClaudeMCPGetOutput(desired.Identity)
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
	if len(args) < 3 || args[0] != "mcp" {
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
	switch args[1] {
	case "get":
		if len(args) != 3 {
			return claudeCommandResult{Err: errors.New("unexpected get command"), Started: true}
		}
		name := args[2]
		for _, scope := range []string{"local", "project", "user"} {
			if output, exists := f.servers[scope][name]; exists {
				return claudeCommandResult{Output: output, Started: true}
			}
		}
		_, exists := f.servers["user"][name]
		if !exists {
			return claudeCommandResult{Output: "Error: MCP server '" + name + "' not found", Err: errors.New("exit status 1"), Started: true}
		}
	case "remove":
		scope, name := args[3], args[4]
		delete(f.servers[scope], name)
		return claudeCommandResult{Started: true}
	case "add":
		scope, name := args[3], args[4]
		f.servers[scope][name] = f.addOutput
		f.added = true
		return claudeCommandResult{Started: true}
	default:
		return claudeCommandResult{Err: errors.New("unexpected command"), Started: true}
	}
	return claudeCommandResult{Err: errors.New("unexpected command"), Started: true}
}

func teammateClaudeMCPGetOutput(name string) string {
	return name + ":\r\n  Scope: User config\r\n  Status: Connected\r\n  Command: C:\\Users\\teammate\\hive-daemon.exe\r\n"
}

func containsScopeUser(args []string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--scope" && args[index+1] == "user" {
			return true
		}
	}
	return false
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
