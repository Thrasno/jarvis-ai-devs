package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// MergeJSON performs a deep merge of patch into base.
//
// Rules:
//   - For objects (map[string]any): patch keys are merged into base recursively.
//     The special key "hive" (Jarvis-managed) is always overwritten by patch.
//   - For arrays ([]any): patch items are appended if not already present.
//     Presence is determined by matching on stable "name" identity for objects.
//   - All other scalar values: patch wins.
//   - Missing keys in base are added from patch.
//   - Existing base keys NOT in patch are preserved unchanged.
//
// Returns indented JSON (2-space) for human-readable config files.
func MergeJSON(base, patch []byte) ([]byte, error) {
	if len(base) == 0 {
		base = []byte("{}")
	}

	baseValue, err := parseOrderedJSON(base)
	if err != nil {
		return nil, fmt.Errorf("parse base JSON: %w", err)
	}

	patchValue, err := parseOrderedJSON(patch)
	if err != nil {
		return nil, fmt.Errorf("parse patch JSON: %w", err)
	}

	if baseValue.object == nil || patchValue.object == nil {
		return nil, fmt.Errorf("merge JSON: base and patch must be objects")
	}
	result := orderedValue{object: deepMergeOrdered(baseValue.object, patchValue.object, false)}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal merged JSON: %w", err)
	}
	return out, nil
}

type orderedValue struct {
	object *orderedObject
	array  []orderedValue
	scalar any
}

type orderedObject struct {
	pairs []orderedPair
}

type orderedPair struct {
	key   string
	value orderedValue
}

func parseOrderedJSON(data []byte) (orderedValue, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeOrderedValue(dec)
	if err != nil {
		return orderedValue{}, err
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err != nil {
			return orderedValue{}, err
		}
		return orderedValue{}, fmt.Errorf("unexpected trailing token %v", tok)
	}
	return value, nil
}

func decodeOrderedValue(dec *json.Decoder) (orderedValue, error) {
	tok, err := dec.Token()
	if err != nil {
		return orderedValue{}, err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			obj := &orderedObject{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return orderedValue{}, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return orderedValue{}, fmt.Errorf("object key is %T, want string", keyTok)
				}
				value, err := decodeOrderedValue(dec)
				if err != nil {
					return orderedValue{}, err
				}
				obj.set(key, value)
			}
			if end, err := dec.Token(); err != nil || end != json.Delim('}') {
				if err != nil {
					return orderedValue{}, err
				}
				return orderedValue{}, fmt.Errorf("unexpected object end %v", end)
			}
			return orderedValue{object: obj}, nil
		case '[':
			arr := []orderedValue{}
			for dec.More() {
				value, err := decodeOrderedValue(dec)
				if err != nil {
					return orderedValue{}, err
				}
				arr = append(arr, value)
			}
			if end, err := dec.Token(); err != nil || end != json.Delim(']') {
				if err != nil {
					return orderedValue{}, err
				}
				return orderedValue{}, fmt.Errorf("unexpected array end %v", end)
			}
			return orderedValue{array: arr}, nil
		}
	}
	return orderedValue{scalar: tok}, nil
}

func (o *orderedObject) index(key string) int {
	for i, pair := range o.pairs {
		if pair.key == key {
			return i
		}
	}
	return -1
}

func (o *orderedObject) get(key string) (orderedValue, bool) {
	idx := o.index(key)
	if idx < 0 {
		return orderedValue{}, false
	}
	return o.pairs[idx].value, true
}

func (o *orderedObject) set(key string, value orderedValue) {
	idx := o.index(key)
	if idx >= 0 {
		o.pairs[idx].value = value
		return
	}
	o.pairs = append(o.pairs, orderedPair{key: key, value: value})
}

func (v orderedValue) MarshalJSON() ([]byte, error) {
	if v.object != nil {
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, pair := range v.object.pairs {
			if i > 0 {
				buf.WriteByte(',')
			}
			key, err := json.Marshal(pair.key)
			if err != nil {
				return nil, err
			}
			value, err := json.Marshal(pair.value)
			if err != nil {
				return nil, err
			}
			buf.Write(key)
			buf.WriteByte(':')
			buf.Write(value)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	}
	if v.array != nil {
		return json.Marshal(v.array)
	}
	return json.Marshal(v.scalar)
}

func deepMergeOrdered(dst, src *orderedObject, inPermission bool) *orderedObject {
	result := &orderedObject{pairs: make([]orderedPair, len(dst.pairs))}
	copy(result.pairs, dst.pairs)

	for _, srcPair := range src.pairs {
		k := srcPair.key
		srcVal := srcPair.value
		mergePermission := inPermission || k == "permission"

		if !mergePermission && (k == "hive" || k == "context7") {
			result.set(k, srcVal)
			continue
		}

		dstVal, exists := result.get(k)
		if !exists {
			if mergePermission && isGeneratedHiveAllow(srcVal.toAny()) && hasStrictHiveWildcardGuardrailOrdered(result, k) {
				continue
			}
			result.set(k, srcVal)
			continue
		}

		if srcVal.object != nil && dstVal.object != nil {
			result.set(k, orderedValue{object: deepMergeOrdered(dstVal.object, srcVal.object, mergePermission)})
			continue
		}

		if srcVal.array != nil && dstVal.array != nil {
			result.set(k, orderedValue{array: mergeOrderedArrays(dstVal.array, srcVal.array)})
			continue
		}

		if mergePermission && isStrictPermissionValue(dstVal.toAny()) {
			continue
		}

		result.set(k, srcVal)
	}

	return result
}

func hasStrictHiveWildcardGuardrailOrdered(permission *orderedObject, key string) bool {
	if !isExactHiveMemPermissionKey(key) {
		return false
	}
	for _, wildcard := range []string{"hive_mem_*", "hive_*"} {
		if value, ok := permission.get(wildcard); ok && isStrictPermissionValue(value.toAny()) {
			return true
		}
	}
	return false
}

func mergeOrderedArrays(dst, src []orderedValue) []orderedValue {
	result := make([]orderedValue, len(dst))
	copy(result, dst)

	for _, srcItem := range src {
		if idx := findOrderedArrayIdentity(result, srcItem); idx >= 0 {
			if result[idx].object != nil && srcItem.object != nil {
				result[idx] = orderedValue{object: deepMergeOrdered(result[idx].object, srcItem.object, false)}
			}
			continue
		}
		if !orderedArrayContains(result, srcItem) {
			result = append(result, srcItem)
		}
	}
	return result
}

func findOrderedArrayIdentity(arr []orderedValue, item orderedValue) int {
	if item.object == nil {
		return -1
	}
	itemVal, ok := item.object.get("name")
	if !ok {
		return -1
	}
	for i, existing := range arr {
		if existing.object == nil {
			continue
		}
		existingVal, ok := existing.object.get("name")
		if ok && reflect.DeepEqual(itemVal.toAny(), existingVal.toAny()) {
			return i
		}
	}
	return -1
}

func orderedArrayContains(arr []orderedValue, item orderedValue) bool {
	for _, existing := range arr {
		if reflect.DeepEqual(existing.toAny(), item.toAny()) {
			return true
		}
	}
	return false
}

func (v orderedValue) toAny() any {
	if v.object != nil {
		m := make(map[string]any, len(v.object.pairs))
		for _, pair := range v.object.pairs {
			m[pair.key] = pair.value.toAny()
		}
		return m
	}
	if v.array != nil {
		arr := make([]any, len(v.array))
		for i, value := range v.array {
			arr[i] = value.toAny()
		}
		return arr
	}
	return v.scalar
}

// deepMerge recursively merges src into dst.
// dst is NOT mutated — a new map is returned.
// For Jarvis-owned keys ("hive", "context7"), src always wins (full overwrite, no deep merge).
func deepMerge(dst, src map[string]any) map[string]any {
	return deepMergeWithContext(dst, src, false)
}

func deepMergeWithContext(dst, src map[string]any, inPermission bool) map[string]any {
	result := make(map[string]any, len(dst))

	// Copy all dst keys first
	for k, v := range dst {
		result[k] = v
	}

	// Apply src keys
	for k, srcVal := range src {
		mergePermission := inPermission || k == "permission"

		// Jarvis-owned keys are always overwritten unconditionally, except inside
		// permission trees where user-owned local guardrails must win.
		if !mergePermission && (k == "hive" || k == "context7") {
			result[k] = srcVal
			continue
		}

		dstVal, exists := result[k]
		if !exists {
			if mergePermission && isGeneratedHiveAllow(srcVal) && hasStrictHiveWildcardGuardrail(result, k) {
				continue
			}
			// New key from patch — add it
			result[k] = srcVal
			continue
		}

		// Both exist — merge by type
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)

		if srcIsMap && dstIsMap {
			result[k] = deepMergeWithContext(dstMap, srcMap, mergePermission)
			continue
		}

		srcArr, srcIsArr := srcVal.([]any)
		dstArr, dstIsArr := dstVal.([]any)

		if srcIsArr && dstIsArr {
			result[k] = mergeArrays(dstArr, srcArr)
			continue
		}

		if mergePermission && isStrictPermissionValue(dstVal) {
			// Permission entries are local guardrails. Generated defaults fill gaps
			// but must not weaken stricter user-owned ask/deny values on rerun.
			// Existing allow values may still be tightened by generated ask/deny rules.
			continue
		}

		// Scalar: patch wins
		result[k] = srcVal
	}

	return result
}

func isGeneratedHiveAllow(value any) bool {
	permission, ok := value.(string)
	return ok && permission == "allow"
}

func hasStrictHiveWildcardGuardrail(permission map[string]any, key string) bool {
	if !isExactHiveMemPermissionKey(key) {
		return false
	}
	for _, wildcard := range []string{"hive_mem_*", "hive_*"} {
		if isStrictPermissionValue(permission[wildcard]) {
			return true
		}
	}
	return false
}

func exactHiveMemPermissionKeys() []string {
	return []string{"hive_mem_search", "hive_mem_get_observation", "hive_mem_save", "hive_mem_context", "hive_mem_session_summary"}
}

func isExactHiveMemPermissionKey(key string) bool {
	for _, exactKey := range exactHiveMemPermissionKeys() {
		if key == exactKey {
			return true
		}
	}
	return false
}

func isStrictPermissionValue(value any) bool {
	permission, ok := value.(string)
	if !ok {
		return false
	}
	return permission == "ask" || permission == "deny"
}

// mergeArrays appends items from src that are not already present in dst.
// Identity is determined by matching stable "name" fields for objects, or by
// deep equality for scalars/arrays/objects without identity.
func mergeArrays(dst, src []any) []any {
	result := make([]any, len(dst))
	copy(result, dst)

	for _, srcItem := range src {
		if idx := findArrayIdentity(result, srcItem); idx >= 0 {
			if merged, ok := mergeArrayItems(result[idx], srcItem); ok {
				result[idx] = merged
			}
			continue
		}
		if !arrayContains(result, srcItem) {
			result = append(result, srcItem)
		}
	}

	return result
}

func mergeArrayItems(dst, src any) (any, bool) {
	dstMap, dstIsMap := dst.(map[string]any)
	srcMap, srcIsMap := src.(map[string]any)
	if !dstIsMap || !srcIsMap {
		return dst, false
	}
	return deepMergeNamedArrayItem(dstMap, srcMap), true
}

func deepMergeNamedArrayItem(dst, src map[string]any) map[string]any {
	result := make(map[string]any, len(dst))
	for k, v := range dst {
		result[k] = v
	}
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = srcVal
			continue
		}
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			result[k] = deepMergeWithContext(dstMap, srcMap, k == "permission")
			continue
		}
		srcArr, srcIsArr := srcVal.([]any)
		dstArr, dstIsArr := dstVal.([]any)
		if srcIsArr && dstIsArr {
			result[k] = mergeArrays(dstArr, srcArr)
			continue
		}
		result[k] = srcVal
	}
	return result
}

func findArrayIdentity(arr []any, item any) int {
	itemMap, itemIsMap := item.(map[string]any)
	if !itemIsMap {
		return -1
	}
	for i, existing := range arr {
		existingMap, existingIsMap := existing.(map[string]any)
		if !existingIsMap {
			continue
		}
		itemVal, iOk := itemMap["name"]
		existingVal, eOk := existingMap["name"]
		if iOk && eOk && reflect.DeepEqual(itemVal, existingVal) {
			return i
		}
	}
	return -1
}

// arrayContains returns true if arr contains an item equal to item.
func arrayContains(arr []any, item any) bool {
	for _, existing := range arr {
		if reflect.DeepEqual(existing, item) {
			return true
		}
	}

	return false
}
