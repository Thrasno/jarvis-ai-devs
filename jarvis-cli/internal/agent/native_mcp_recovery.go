package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// NativeMCPDefinition describes the expected, provenance-bound native MCP.
// Jarvis-managed MCPs are always installed at the OS-user-global Claude scope.
type NativeMCPDefinition struct {
	Identity            string
	Scope               string
	SchemaVersion       string
	AddArgs             []string
	ExpectedFingerprint string
}

// NativeMCPPhase records a native MCP operation boundary.
type NativeMCPPhase string

const NativeMCPSnapshotted NativeMCPPhase = "snapshotted"

const (
	NativeMCPSkipped   NativeMCPPhase = "skipped"
	NativeMCPInspected NativeMCPPhase = "inspected"
	NativeMCPRemoved   NativeMCPPhase = "removed"
	NativeMCPAdded     NativeMCPPhase = "added"
	NativeMCPVerifying NativeMCPPhase = "verifying"
	NativeMCPVerified  NativeMCPPhase = "verified"
)

const nativeMCPFixForwardGuidance = "correct the native MCP error and rerun Install/Reconfigure"

const nativeMCPUserScope = "user"

const (
	claudeHiveIdentity        = "hive"
	claudeContext7Identity    = "context7"
	claudeMCPDefinitionSchema = "v1"
	context7MCPURL            = "https://mcp.context7.com/mcp"
)

// ClaudeUserMCPDefinitions returns the canonical user-scoped Hive and Context7
// definitions used for managed Claude reconciliation.
func ClaudeUserMCPDefinitions(hiveDaemonPath string) (NativeMCPDefinition, NativeMCPDefinition, error) {
	hiveDaemonPath = strings.TrimSpace(hiveDaemonPath)
	if !validHiveDaemonPath(hiveDaemonPath) {
		return NativeMCPDefinition{}, NativeMCPDefinition{}, errors.New("Hive daemon executable is unavailable; repair installation and rerun Install/Reconfigure")
	}
	hive := NativeMCPDefinition{
		Identity:            claudeHiveIdentity,
		Scope:               nativeMCPUserScope,
		SchemaVersion:       claudeMCPDefinitionSchema,
		AddArgs:             []string{"mcp", "add", "--transport", "stdio", "--scope", nativeMCPUserScope, claudeHiveIdentity, "--", hiveDaemonPath},
		ExpectedFingerprint: nativeMCPFingerprint(`{"type":"stdio","command":` + strconv.Quote(hiveDaemonPath) + `,"args":[]}`),
	}
	context7 := NativeMCPDefinition{
		Identity:            claudeContext7Identity,
		Scope:               nativeMCPUserScope,
		SchemaVersion:       claudeMCPDefinitionSchema,
		AddArgs:             []string{"mcp", "add", "--transport", "http", "--scope", nativeMCPUserScope, claudeContext7Identity, context7MCPURL},
		ExpectedFingerprint: nativeMCPFingerprint(`{"type":"http","url":` + strconv.Quote(context7MCPURL) + `}`),
	}
	return hive, context7, nil
}

// NativeMCPInventory is safe to persist or report: it contains no command
// output, command arguments, or configuration values.
type NativeMCPInventory struct {
	Identity    string
	Fingerprint string
}

// NativeMCPJournal contains only secret-safe lifecycle evidence.
type NativeMCPJournal struct {
	Phase   NativeMCPPhase
	Managed []NativeMCPInventory
}

// Diagnostics is suitable for ordinary reports because it exposes identities
// and content fingerprints only.
func (j NativeMCPJournal) Diagnostics() string {
	managed := make([]string, 0, len(j.Managed))
	for _, item := range j.Managed {
		managed = append(managed, item.Identity+":"+item.Fingerprint)
	}
	return "native MCP phase=" + string(j.Phase) + " managed=" + strings.Join(managed, ",")
}

// NativeMCPResult is transient, secret-safe replacement evidence. It never
// contains native output, add arguments, or prior configuration values.
type NativeMCPResult struct {
	Phase         NativeMCPPhase
	TargetName    string
	FixedLocation string
	ErrorCategory string
	ErrorCode     string
	Guidance      string
}

func (r NativeMCPResult) Diagnostics() string {
	return "native MCP phase=" + string(r.Phase) + " target=" + r.TargetName + " location=" + r.FixedLocation + " error=" + r.ErrorCategory + "/" + r.ErrorCode
}

// NativeMCPManager inventories native Claude MCP state through an injected
// runner. It neither resolves local paths nor invokes real commands itself.
type NativeMCPManager struct{ run claudeCommandRunner }

type nativeMCPManifest struct {
	SchemaVersion       string `json:"schema_version"`
	ManagedIdentity     string `json:"managed_identity"`
	Scope               string `json:"scope"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

// Snapshot classifies desired identities without mutation. A missing identity
// is creatable; an existing identity is managed only when its structured
// provenance and independently supplied expected fingerprint both match the
// observed configuration digest.
func (m NativeMCPManager) Snapshot(definitions []NativeMCPDefinition) (*NativeMCPJournal, error) {
	journal := &NativeMCPJournal{Phase: NativeMCPSnapshotted}
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := validateNativeMCPDefinition(definition); err != nil {
			return nil, err
		}
		if _, duplicate := seen[definition.Identity]; duplicate {
			return nil, fmt.Errorf("native MCP %s: duplicate desired identity", definition.Identity)
		}
		seen[definition.Identity] = struct{}{}

		result, retried := m.getNativeMCP(definition.Identity)
		if result.Err != nil {
			if isMissingClaudeMCP(result, definition.Identity) {
				continue
			}
			return nil, fmt.Errorf("native MCP %s failed (inspection/%s): %s", definition.Identity, nativeMCPCommandErrorCode(result), nativeMCPFixForwardGuidance)
		}
		if retried && !nativeMCPGetRecordUsable(result, definition.Identity) {
			return nil, fmt.Errorf("native MCP %s failed (inspection/invalid-record): %s", definition.Identity, nativeMCPFixForwardGuidance)
		}
		if code, user := nativeMCPGetUserScope(result.Output, definition.Identity); !user {
			return nil, fmt.Errorf("native MCP %s failed (wrong-scope/%s): %s", definition.Identity, code, nativeMCPFixForwardGuidance)
		}
		journal.Managed = append(journal.Managed, NativeMCPInventory{Identity: definition.Identity, Fingerprint: definition.ExpectedFingerprint})
	}
	return journal, nil
}

// Replace converges wizard-reserved names at Claude's fixed OS-user-global
// scope. It has no rollback or restoration path: failures stop with fix-forward
// guidance and never inspect or modify local or project MCP entries.
func (m NativeMCPManager) Replace(definitions []NativeMCPDefinition) (*NativeMCPResult, error) {
	if len(definitions) == 0 {
		return &NativeMCPResult{Phase: NativeMCPSkipped}, nil
	}
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		result := newNativeMCPResult(definition.Identity)
		if err := validateNativeMCPMutationDefinition(definition); err != nil {
			return m.replaceFailure(result, "definition", "invalid")
		}
		if _, exists := seen[definition.Identity]; exists {
			return m.replaceFailure(result, "definition", "duplicate-name")
		}
		seen[definition.Identity] = struct{}{}
	}
	var completed *NativeMCPResult
	for _, definition := range definitions {
		result := newNativeMCPResult(definition.Identity)
		command, retried := m.getNativeMCP(definition.Identity)
		if retried && command.Err == nil && !nativeMCPGetRecordUsable(command, definition.Identity) {
			return m.replaceFailure(result, "inspection", "invalid-record")
		}
		if command.Err == nil {
			if code, user := nativeMCPGetUserScope(command.Output, definition.Identity); !user {
				return m.replaceFailure(result, "wrong-scope", code)
			}
			result.Phase = NativeMCPRemoved
			command = m.runner()("claude", "mcp", "remove", "--scope", nativeMCPUserScope, definition.Identity)
			if command.Err != nil {
				return m.replaceCommandFailure(result, NativeMCPRemoved, command)
			}
		} else if !isMissingClaudeMCP(command, definition.Identity) {
			return m.replaceFailure(result, "inspection", nativeMCPCommandErrorCode(command))
		}
		result.Phase = NativeMCPAdded
		if command := m.runner()("claude", definition.AddArgs...); command.Err != nil {
			return m.replaceCommandFailure(result, NativeMCPAdded, command)
		}
		result.Phase = NativeMCPVerifying
		command, retried = m.getNativeMCP(definition.Identity)
		if retried && command.Err == nil && !nativeMCPGetRecordUsable(command, definition.Identity) {
			return m.replaceFailure(result, "verification", "invalid-record")
		}
		if command.Err != nil {
			if isMissingClaudeMCP(command, definition.Identity) {
				return m.replaceFailure(result, "verification", "user-scope-presence")
			}
			return m.replaceFailure(result, "verification", nativeMCPCommandErrorCode(command))
		}
		if code, user := nativeMCPGetUserScope(command.Output, definition.Identity); !user {
			return m.replaceFailure(result, "wrong-scope", code)
		}
		result.Phase = NativeMCPVerified
		completed = result
	}
	return completed, nil
}

func newNativeMCPResult(identity string) *NativeMCPResult {
	return &NativeMCPResult{
		Phase:         NativeMCPInspected,
		TargetName:    identity,
		FixedLocation: "claude --scope " + nativeMCPUserScope,
		Guidance:      nativeMCPFixForwardGuidance,
	}
}

func (m NativeMCPManager) replaceCommandFailure(result *NativeMCPResult, phase NativeMCPPhase, command claudeCommandResult) (*NativeMCPResult, error) {
	result.Phase = phase
	return m.replaceFailure(result, "native-command", nativeMCPCommandErrorCode(command))
}

func (m NativeMCPManager) replaceFailure(result *NativeMCPResult, category, code string) (*NativeMCPResult, error) {
	result.ErrorCategory, result.ErrorCode = category, code
	return result, fmt.Errorf("native MCP %s failed (%s/%s): %s", result.Phase, category, code, result.Guidance)
}

func nativeMCPCommandErrorCode(result claudeCommandResult) string {
	switch {
	case errors.Is(result.Err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(result.Err, os.ErrPermission):
		return "permission"
	case !result.Started:
		return "not-started"
	default:
		return "nonzero-exit"
	}
}

func nativeMCPGetRecordUsable(result claudeCommandResult, identity string) bool {
	if !result.Started || errors.Is(result.Err, os.ErrNotExist) || errors.Is(result.Err, os.ErrPermission) || errors.Is(result.Err, context.DeadlineExceeded) {
		return false
	}
	_, user := nativeMCPGetUserScope(result.Output, identity)
	return user
}

func (m NativeMCPManager) getNativeMCP(identity string) (claudeCommandResult, bool) {
	run := m.runner()
	result := run("claude", "mcp", "get", identity)
	if result.Err != nil && nativeMCPGetRecordUsable(result, identity) {
		return run("claude", "mcp", "get", identity), true
	}
	return result, false
}

func nativeMCPGetUserScope(output, identity string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(stripANSI(output), "\r\n", "\n"), "\n")
	first := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			first = strings.TrimSpace(line)
			break
		}
	}
	if first != identity+":" {
		return "identity", false
	}
	scope := ""
	for _, line := range lines {
		key, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "scope") {
			continue
		}
		fields := strings.Fields(strings.ToLower(value))
		if len(fields) == 0 {
			return "missing", false
		}
		current := fields[0]
		if current != "user" && current != "local" && current != "project" {
			return "unknown", false
		}
		if scope != "" && scope != current {
			return "conflicting", false
		}
		scope = current
	}
	if scope == "" {
		return "missing", false
	}
	return scope, scope == nativeMCPUserScope
}

func stripANSI(value string) string {
	var clean strings.Builder
	for index := 0; index < len(value); {
		if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '[' {
			index += 2
			for index < len(value) {
				character := value[index]
				index++
				if character >= 0x40 && character <= 0x7e {
					break
				}
			}
			continue
		}
		clean.WriteByte(value[index])
		index++
	}
	return clean.String()
}

func (m NativeMCPManager) runner() claudeCommandRunner {
	if m.run != nil {
		return m.run
	}
	return runNativeMCPInventoryCommand
}

func validateNativeMCPDefinition(definition NativeMCPDefinition) error {
	if definition.Identity == "" || definition.Scope != nativeMCPUserScope || definition.SchemaVersion == "" || definition.ExpectedFingerprint == "" {
		return errors.New("native MCP definition is incomplete")
	}
	return nil
}

func validateNativeMCPMutationDefinition(definition NativeMCPDefinition) error {
	if definition.Identity == "" || definition.Scope != nativeMCPUserScope || !nativeMCPAddArgsMatchDefinition(definition.AddArgs, definition.Identity) {
		return fmt.Errorf("native MCP %s: add arguments do not match definition", definition.Identity)
	}
	return nil
}

func nativeMCPAddArgsMatchDefinition(args []string, identity string) bool {
	if len(args) < 5 || args[0] != "mcp" || args[1] != "add" {
		return false
	}
	scopeIndex := 2
	if args[2] == "--transport" {
		if len(args) < 7 || (args[3] != "stdio" && args[3] != "http") {
			return false
		}
		scopeIndex = 4
	}
	return args[scopeIndex] == "--scope" && args[scopeIndex+1] == nativeMCPUserScope && args[scopeIndex+2] == identity
}

func validateNativeMCPProvenance(output string, definition NativeMCPDefinition) (string, bool) {
	var document map[string]json.RawMessage
	if json.Unmarshal([]byte(output), &document) != nil {
		return "", false
	}
	provenance, found := document["jarvis_provenance"]
	if !found {
		return "", false
	}
	var manifest nativeMCPManifest
	if json.Unmarshal(provenance, &manifest) != nil {
		return "", false
	}
	delete(document, "jarvis_provenance")
	configuration, err := json.Marshal(document)
	if err != nil {
		return "", false
	}
	fingerprint := nativeMCPFingerprint(string(configuration))
	return fingerprint, manifest.ManagedIdentity == definition.Identity &&
		manifest.Scope == definition.Scope &&
		manifest.SchemaVersion == definition.SchemaVersion &&
		manifest.ConfigurationSHA256 == fingerprint &&
		fingerprint == definition.ExpectedFingerprint
}

func nativeMCPFingerprint(configuration string) string {
	var value any
	if json.Unmarshal([]byte(configuration), &value) == nil {
		if normalized, err := json.Marshal(value); err == nil {
			configuration = string(normalized)
		}
	}
	sum := sha256.Sum256([]byte(configuration))
	return "sha256:" + hex.EncodeToString(sum[:])
}
