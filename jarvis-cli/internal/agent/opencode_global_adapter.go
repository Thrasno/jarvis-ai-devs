package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

// OpenCodeConfigFS keeps the global-config reader injectable. This adapter never
// resolves a home directory or writes a configuration file.
type OpenCodeConfigFS interface {
	ReadFile(string) ([]byte, error)
}

type osFS struct{}

func (osFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// OpenCodeManagedMCPs is deliberately restricted to Jarvis' user-global names.
// Values are complete JSON objects supplied by the caller's desired state.
type OpenCodeManagedMCPs map[string]string

// OpenCodeGlobalAdapter reads and renders only the fixed user-global OpenCode
// configuration beneath an injected root. It has no mutation capability.
type OpenCodeGlobalAdapter struct {
	fs   OpenCodeConfigFS
	root string
}

func NewOpenCodeGlobalAdapter(fsys OpenCodeConfigFS, root string) OpenCodeGlobalAdapter {
	return OpenCodeGlobalAdapter{fs: fsys, root: filepath.Clean(root)}
}

func (a OpenCodeGlobalAdapter) Render(managed OpenCodeManagedMCPs) (RenderedManagedOutput, error) {
	doc, raw, exists, err := a.readDocument()
	if err != nil {
		return RenderedManagedOutput{}, err
	}
	if err := validateOpenCodeManagedNames(managed); err != nil {
		return RenderedManagedOutput{}, err
	}
	if exists && hasJarvisManagedMCP(doc) {
		return RenderedManagedOutput{}, errors.New("OpenCode managed ownership is ambiguous; repair it and rerun Install/Reconfigure")
	}
	if err := mergeOpenCodeManagedMCPs(doc, managed); err != nil {
		return RenderedManagedOutput{}, err
	}
	rendered, err := json.Marshal(doc)
	if err != nil {
		return RenderedManagedOutput{}, errors.New("OpenCode configuration cannot be rendered; repair it and rerun Install/Reconfigure")
	}
	output := RenderedManagedOutput{Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, Bytes: rendered}
	if exists {
		// A caller may inspect merged bytes, but PR5A3 must block any write to
		// an existing global config until durable ownership is supplied.
		output.Existing = &reconcile.Artifact{Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, Bytes: append([]byte(nil), raw...)}
	}
	return output, nil
}

// RenderWithProvenance produces a PR5A3-ready output only after the supplied
// evidence proves the exact existing global artifact and managed names. Proven
// Jarvis entries may be replaced, but the evidence is never fabricated or used
// to authorize a name absent from the proven artifact.
func (a OpenCodeGlobalAdapter) RenderWithProvenance(managed OpenCodeManagedMCPs, provenance *reconcile.Provenance) (RenderedManagedOutput, error) {
	inventory, err := a.InventoryWithProvenance(provenance)
	if err != nil {
		return RenderedManagedOutput{}, err
	}
	if len(inventory) != 1 {
		return RenderedManagedOutput{}, errors.New("OpenCode managed provenance is required; repair it and rerun Install/Reconfigure")
	}
	doc := map[string]json.RawMessage{}
	if unmarshalErr := json.Unmarshal(inventory[0].Bytes, &doc); unmarshalErr != nil {
		return RenderedManagedOutput{}, errors.New("OpenCode global configuration is malformed; repair it and rerun Install/Reconfigure")
	}
	if validateErr := validateOpenCodeManagedNames(managed); validateErr != nil {
		return RenderedManagedOutput{}, validateErr
	}
	if !openCodeManagedNamesAreOwned(doc, managed) {
		return RenderedManagedOutput{}, errors.New("OpenCode managed ownership is ambiguous; repair it and rerun Install/Reconfigure")
	}
	output := RenderedManagedOutput{Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation}
	if openCodeManagedMCPsMatch(doc, managed) {
		output.Bytes = append([]byte(nil), inventory[0].Bytes...)
	} else {
		if mergeErr := mergeOpenCodeManagedMCPs(doc, managed); mergeErr != nil {
			return RenderedManagedOutput{}, mergeErr
		}
		output.Bytes, err = json.Marshal(doc)
		if err != nil {
			return RenderedManagedOutput{}, errors.New("OpenCode configuration cannot be rendered; repair it and rerun Install/Reconfigure")
		}
	}
	output.Existing = &inventory[0]
	return output, nil
}

// Inventory returns only the fixed Jarvis-owned global artifact. User MCP names
// and any other configuration keys are not inventory candidates.
func (a OpenCodeGlobalAdapter) Inventory() ([]reconcile.Artifact, error) {
	doc, _, exists, err := a.readDocument()
	if err != nil || !exists {
		return nil, err
	}
	if hasJarvisManagedMCP(doc) {
		return nil, errors.New("OpenCode managed ownership requires provenance; repair it and rerun Install/Reconfigure")
	}
	return nil, nil
}

// InventoryWithProvenance accepts a current global artifact only when its
// supplied durable evidence binds every required value to the bytes on disk.
// It never creates, upgrades, or guesses provenance.
func (a OpenCodeGlobalAdapter) InventoryWithProvenance(provenance *reconcile.Provenance) ([]reconcile.Artifact, error) {
	doc, raw, exists, err := a.readDocument()
	if err != nil || !exists {
		return nil, err
	}
	if !hasJarvisManagedMCP(doc) {
		return nil, nil
	}
	if provenance == nil || provenance.Version != "v1" || provenance.ManagedIdentity != openCodeGlobalConfigIdentity ||
		provenance.Location != openCodeGlobalConfigLocation || provenance.ManifestDigest != managedOutputDigest(raw) {
		return nil, errors.New("OpenCode managed provenance does not match; repair it and rerun Install/Reconfigure")
	}
	return []reconcile.Artifact{{
		Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation,
		Bytes: append([]byte(nil), raw...), Provenance: provenance,
	}}, nil
}

func (a OpenCodeGlobalAdapter) readDocument() (map[string]json.RawMessage, []byte, bool, error) {
	if a.fs == nil || strings.TrimSpace(a.root) == "" {
		return nil, nil, false, errors.New("OpenCode configuration adapter is unavailable")
	}
	raw, err := a.fs.ReadFile(filepath.Join(a.root, filepath.FromSlash(openCodeGlobalConfigLocation)))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, errors.New("OpenCode global configuration cannot be read; repair it and rerun Install/Reconfigure")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, false, errors.New("OpenCode global configuration is malformed; repair it and rerun Install/Reconfigure")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return nil, nil, false, errors.New("OpenCode global configuration is malformed; repair it and rerun Install/Reconfigure")
	}
	return doc, raw, true, nil
}

func validateOpenCodeManagedNames(managed OpenCodeManagedMCPs) error {
	if len(managed) == 0 {
		return errors.New("OpenCode managed MCP desired state is required")
	}
	for name, raw := range managed {
		if !isJarvisOpenCodeMCPName(name) {
			return fmt.Errorf("OpenCode MCP %q is outside Jarvis ownership", name)
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
			return errors.New("OpenCode managed MCP desired state is malformed")
		}
	}
	return nil
}

func mergeOpenCodeManagedMCPs(doc map[string]json.RawMessage, managed OpenCodeManagedMCPs) error {
	mcp := map[string]json.RawMessage{}
	if raw, found := doc["mcp"]; found {
		if err := json.Unmarshal(raw, &mcp); err != nil || mcp == nil {
			return errors.New("OpenCode global MCP configuration is malformed; repair it and rerun Install/Reconfigure")
		}
	}
	names := make([]string, 0, len(managed))
	for name := range managed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		mcp[name] = json.RawMessage(managed[name])
	}
	for name := range mcp {
		if isJarvisOpenCodeMCPName(name) {
			if _, desired := managed[name]; !desired {
				delete(mcp, name)
			}
		}
	}
	encoded, err := json.Marshal(mcp)
	if err != nil {
		return errors.New("OpenCode global MCP configuration cannot be rendered; repair it and rerun Install/Reconfigure")
	}
	doc["mcp"] = encoded
	return nil
}

func hasJarvisManagedMCP(doc map[string]json.RawMessage) bool {
	raw, found := doc["mcp"]
	if !found {
		return false
	}
	var mcp map[string]json.RawMessage
	if json.Unmarshal(raw, &mcp) != nil {
		return true
	}
	for name := range mcp {
		if isJarvisOpenCodeMCPName(name) {
			return true
		}
	}
	return false
}

func openCodeManagedMCPsMatch(doc map[string]json.RawMessage, managed OpenCodeManagedMCPs) bool {
	raw, found := doc["mcp"]
	if !found {
		return false
	}
	var mcp map[string]json.RawMessage
	if json.Unmarshal(raw, &mcp) != nil {
		return false
	}
	for name, want := range managed {
		got, found := mcp[name]
		if !found || !jsonEqual(got, []byte(want)) {
			return false
		}
	}
	for name := range mcp {
		if isJarvisOpenCodeMCPName(name) {
			if _, desired := managed[name]; !desired {
				return false
			}
		}
	}
	return true
}

func openCodeManagedNamesAreOwned(doc map[string]json.RawMessage, managed OpenCodeManagedMCPs) bool {
	raw, found := doc["mcp"]
	if !found {
		return false
	}
	var mcp map[string]json.RawMessage
	if json.Unmarshal(raw, &mcp) != nil {
		return false
	}
	for name := range managed {
		if _, found := mcp[name]; !found || !isJarvisOpenCodeMCPName(name) {
			return false
		}
	}
	return true
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func isJarvisOpenCodeMCPName(name string) bool {
	return name == "hive" || name == "context7"
}
