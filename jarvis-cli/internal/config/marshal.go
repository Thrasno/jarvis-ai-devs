package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// keySet describes the config.yaml keys AppConfig spells. A nil value marks a
// leaf; a non-nil value is the key set of a nested struct.
type keySet map[string]keySet

// marshalPreservingUnknownKeys renders cfg as config.yaml without discarding
// keys the struct does not know about.
//
// Two things depend on this. The first is the repository rule that config
// emitters merge rather than clobber: a key some other writer owns must survive
// a save by this one. The second is the migration window. Once the replay fields
// left AppConfig, a machine that has not run state.Migrate yet still carries
// them in config.yaml, and a plain load-then-save would decode into a struct
// with nowhere to put them and write the file back without them, stranding the
// user's persona, skills, agents, scope and phase models before the migration
// ever got to move them.
//
// Keys the struct does spell are authoritative, including by their absence: a
// caller that clears Cloud means to remove the cloud block, and preserving it
// would make that impossible.
func marshalPreservingUnknownKeys(cfg *AppConfig, path string) ([]byte, error) {
	next, err := encodeToMap(cfg)
	if err != nil {
		return nil, err
	}

	existing, err := decodeExistingConfig(path)
	if err != nil {
		return nil, err
	}

	merged := deepMerge(keepUnknownKeys(existing, appConfigKeys()), next)
	data, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return data, nil
}

func encodeToMap(cfg *AppConfig) (map[string]any, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return out, nil
}

// decodeExistingConfig reads the file being replaced. A missing or unreadable
// file is not an error: there is simply nothing to preserve.
func decodeExistingConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	out := map[string]any{}
	if strings.TrimSpace(string(data)) == "" {
		return out, nil
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		// An unparseable file carries nothing worth preserving, and refusing to
		// save over it would leave the machine unable to record anything.
		return map[string]any{}, nil
	}
	return out, nil
}

// keepUnknownKeys strips every key the struct spells, leaving only what another
// writer or an unmigrated release put there.
func keepUnknownKeys(raw map[string]any, known keySet) map[string]any {
	out := map[string]any{}
	for key, value := range raw {
		nested, isKnown := known[key]
		if !isKnown {
			out[key] = value
			continue
		}
		if nested == nil {
			continue
		}
		sub, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if kept := keepUnknownKeys(sub, nested); len(kept) > 0 {
			out[key] = kept
		}
	}
	return out
}

// deepMerge overlays next onto base, recursing into nested mappings so a nested
// preserved key is not lost to a nested written one.
func deepMerge(base, next map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(next))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range next {
		baseMap, baseOK := out[key].(map[string]any)
		nextMap, nextOK := value.(map[string]any)
		if baseOK && nextOK {
			out[key] = deepMerge(baseMap, nextMap)
			continue
		}
		out[key] = value
	}
	return out
}

func appConfigKeys() keySet {
	return keysForType(reflect.TypeOf(AppConfig{}))
}

func keysForType(t reflect.Type) keySet {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	out := keySet{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := yamlKeyName(field)
		if name == "" {
			continue
		}
		out[name] = keysForType(field.Type)
	}
	return out
}

// yamlKeyName returns the config.yaml key a struct field is written under, or
// the empty string when the field is not written as a key of its own.
func yamlKeyName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup("yaml")
	if !ok {
		return strings.ToLower(field.Name)
	}
	name, _, _ := strings.Cut(tag, ",")
	name = strings.TrimSpace(name)
	if name == "-" {
		return ""
	}
	if name == "" {
		// Either an inline field or one relying on the default name. Inline
		// fields have no key of their own, so treating an untagged name as
		// unknown would be wrong; yaml's default is the lowercased field name.
		if strings.Contains(tag, "inline") {
			return ""
		}
		return strings.ToLower(field.Name)
	}
	return name
}
