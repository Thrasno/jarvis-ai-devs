package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NativeMCPDefinition describes the expected, provenance-bound native MCP.
// AddArgs are accepted only for a later mutation slice and never enter the
// inventory or its diagnostics.
type NativeMCPDefinition struct {
	Identity            string
	Scope               string
	SchemaVersion       string
	AddArgs             []string
	ExpectedFingerprint string
}

// NativeMCPPhase records the durable state boundary reached by a journal.
type NativeMCPPhase string

const NativeMCPSnapshotted NativeMCPPhase = "snapshotted"

// NativeMCPInventory is safe to persist or report: it contains no command
// output, command arguments, or configuration values.
type NativeMCPInventory struct {
	Identity    string
	Fingerprint string
}

// NativeMCPJournal contains only secret-safe inventory evidence. Mutation and
// recovery payloads are intentionally deferred to the PR4B recovery slice.
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

		result := m.runner()("claude", "mcp", "get", definition.Identity)
		if result.Err != nil {
			if isMissingClaudeMCP(result, definition.Identity) {
				continue
			}
			return nil, fmt.Errorf("inventory native MCP %s: %w", definition.Identity, result.Err)
		}
		fingerprint, proven := validateNativeMCPProvenance(result.Output, definition)
		if !proven {
			return nil, fmt.Errorf("native MCP %s: ownership is not proven", definition.Identity)
		}
		journal.Managed = append(journal.Managed, NativeMCPInventory{Identity: definition.Identity, Fingerprint: fingerprint})
	}
	return journal, nil
}

func (m NativeMCPManager) runner() claudeCommandRunner {
	if m.run != nil {
		return m.run
	}
	return runCommandCombinedOutput
}

func validateNativeMCPDefinition(definition NativeMCPDefinition) error {
	if definition.Identity == "" || definition.Scope == "" || definition.SchemaVersion == "" || definition.ExpectedFingerprint == "" {
		return errors.New("native MCP definition is incomplete")
	}
	return nil
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
