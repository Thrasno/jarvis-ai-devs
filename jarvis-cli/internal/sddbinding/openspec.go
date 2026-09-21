package sddbinding

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"gopkg.in/yaml.v3"
)

var syncChangeRoot = func(int) error { return nil }

const (
	stateFileName        = "state.yaml"
	bindingKey           = "jarvis_sdd_binding"
	schemaKey            = "schema_version"
	bindingSchemaVersion = "1"
)

// ReadOpenSpec returns the binding in changeDir's OpenSpec state, or nil when unbound.
func ReadOpenSpec(changeDir string) (*Binding, error) {
	root, err := openChangeRoot(changeDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	doc, _, exists, err := readStateDocument(root)
	if err != nil || !exists {
		return nil, err
	}
	binding, _, err := bindingFromDocument(doc)
	return binding, err
}

// AdoptOpenSpec persists requested when state is unbound. Exact replays are no-ops;
// a different persisted selection returns a typed conflict without mutating state.
func AdoptOpenSpec(changeDir string, requested Binding) (Binding, bool, error) {
	if _, err := New(requested.mode, requested.provenance); err != nil {
		return Binding{}, false, err
	}
	root, err := openChangeRoot(changeDir)
	if err != nil {
		return Binding{}, false, err
	}
	defer root.Close()

	doc, state, exists, err := readStateDocument(root)
	if err != nil {
		return Binding{}, false, err
	}
	if binding, resolved, err := resolveExisting(doc, exists, requested); resolved || err != nil {
		return binding, false, err
	}

	unlock, err := root.lock()
	if err != nil {
		return Binding{}, false, err
	}
	defer unlock()
	doc, state, exists, err = readStateDocument(root)
	if err != nil {
		return Binding{}, false, err
	}
	if binding, resolved, err := resolveExisting(doc, exists, requested); resolved || err != nil {
		return binding, false, err
	}
	if !exists {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	appendBinding(doc.Content[0], requested)
	data, err := marshalDocument(doc)
	if err != nil {
		return Binding{}, false, err
	}
	if err := root.replaceState(data, state); err != nil {
		return Binding{}, false, err
	}
	return requested, true, nil
}

func resolveExisting(doc *yaml.Node, exists bool, requested Binding) (Binding, bool, error) {
	if !exists {
		return Binding{}, false, nil
	}
	existing, _, err := bindingFromDocument(doc)
	if err != nil {
		return Binding{}, true, err
	}
	if existing == nil {
		return Binding{}, false, nil
	}
	if *existing == requested {
		return requested, true, nil
	}
	return Binding{}, true, &ConflictError{Existing: *existing, Requested: requested}
}

func readStateDocument(root *changeRoot) (*yaml.Node, stateRecord, bool, error) {
	state, err := root.readState()
	if err != nil || !state.exists {
		return nil, state, state.exists, err
	}
	doc, err := parseDocument(state.data)
	if err != nil {
		return nil, state, true, err
	}
	return doc, state, true, nil
}

func parseDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: YAML parse: %v", ErrInvalidState, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%w: multiple YAML documents", ErrInvalidState)
		}
		return nil, fmt.Errorf("%w: YAML parse: %v", ErrInvalidState, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Tag != "!!map" {
		return nil, fmt.Errorf("%w: root must be a mapping", ErrInvalidState)
	}
	if err := validateUnambiguousYAML(&doc, map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	return &doc, nil
}

func validateUnambiguousYAML(node *yaml.Node, seen map[*yaml.Node]bool) error {
	if node == nil || seen[node] {
		return nil
	}
	seen[node] = true
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := validateUnambiguousYAML(child, seen); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return fmt.Errorf("%w: malformed mapping", ErrInvalidState)
		}
		keys := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind != yaml.ScalarNode || key.Tag == "!!merge" || key.Value == "<<" {
				return fmt.Errorf("%w: ambiguous mapping key", ErrInvalidState)
			}
			identity, err := canonicalScalarKey(key)
			if err != nil {
				return err
			}
			if keys[identity] {
				return fmt.Errorf("%w: duplicate mapping key %q", ErrInvalidState, key.Value)
			}
			keys[identity] = true
			if err := validateUnambiguousYAML(value, seen); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		if node.Alias == nil {
			return fmt.Errorf("%w: invalid YAML alias", ErrInvalidState)
		}
		return validateUnambiguousYAML(node.Alias, seen)
	}
	return nil
}

func canonicalScalarKey(key *yaml.Node) (string, error) {
	var value any
	if err := key.Decode(&value); err != nil {
		return "", fmt.Errorf("%w: invalid mapping key", ErrInvalidState)
	}
	switch number := value.(type) {
	case float64:
		return canonicalFloatKey(number), nil
	case float32:
		return canonicalFloatKey(float64(number)), nil
	default:
		return fmt.Sprintf("%T:%#v", value, value), nil
	}
}

func canonicalFloatKey(value float64) string {
	switch {
	case math.IsNaN(value):
		return "float64:nan"
	case math.IsInf(value, 1):
		return "float64:+inf"
	case math.IsInf(value, -1):
		return "float64:-inf"
	case value == 0:
		return "float64:0"
	default:
		return "float64:" + strconv.FormatFloat(value, 'g', -1, 64)
	}
}

func bindingFromDocument(doc *yaml.Node) (*Binding, *yaml.Node, error) {
	root := doc.Content[0]
	var value *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		if key.Kind == yaml.ScalarNode && key.Value == bindingKey {
			if value != nil {
				return nil, nil, fmt.Errorf("%w: duplicate %s", ErrInvalidState, bindingKey)
			}
			value = root.Content[i+1]
		}
	}
	if value == nil {
		return nil, nil, nil
	}
	binding, err := decodeBinding(value)
	if err != nil {
		return nil, nil, err
	}
	return &binding, value, nil
}

func decodeBinding(node *yaml.Node) (Binding, error) {
	if node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return Binding{}, fmt.Errorf("%w: binding must be a mapping", ErrInvalidState)
	}
	values := map[string]*yaml.Node{}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || (key.Value != schemaKey && key.Value != "mode" && key.Value != "provenance") {
			return Binding{}, fmt.Errorf("%w: malformed binding field", ErrInvalidState)
		}
		if _, exists := values[key.Value]; exists {
			return Binding{}, fmt.Errorf("%w: duplicate binding field %q", ErrInvalidState, key.Value)
		}
		values[key.Value] = value
	}
	version, mode, provenance := values[schemaKey], values["mode"], values["provenance"]
	if version == nil || version.Kind != yaml.ScalarNode || version.Tag != "!!int" || version.Value != bindingSchemaVersion {
		return Binding{}, fmt.Errorf("%w: unsupported binding schema", ErrInvalidState)
	}
	if mode == nil || mode.Kind != yaml.ScalarNode || mode.Tag != "!!str" || provenance == nil || provenance.Kind != yaml.ScalarNode || provenance.Tag != "!!str" {
		return Binding{}, fmt.Errorf("%w: malformed binding fields", ErrInvalidState)
	}
	binding, err := New(sddruntime.StoreMode(mode.Value), provenance.Value)
	if err != nil {
		return Binding{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	return binding, nil
}

func appendBinding(root *yaml.Node, binding Binding) {
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: bindingKey},
		&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: schemaKey},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: bindingSchemaVersion},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "mode"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(binding.mode)},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "provenance"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: binding.provenance},
		}},
	)
}

func marshalDocument(doc *yaml.Node) ([]byte, error) {
	var data bytes.Buffer
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode OpenSpec binding state: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encode OpenSpec binding state: %w", err)
	}
	return data.Bytes(), nil
}
