package agent

import (
	"encoding/json"
	"fmt"
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

	var baseMap map[string]any
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return nil, fmt.Errorf("parse base JSON: %w", err)
	}

	var patchMap map[string]any
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return nil, fmt.Errorf("parse patch JSON: %w", err)
	}

	result := deepMerge(baseMap, patchMap)

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal merged JSON: %w", err)
	}
	return out, nil
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

	if inPermission {
		removeGeneratedHiveAllowsWeakenedByStrictWildcard(result)
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

func removeGeneratedHiveAllowsWeakenedByStrictWildcard(permission map[string]any) {
	for _, wildcard := range []string{"hive_mem_*", "hive_*"} {
		if !isStrictPermissionValue(permission[wildcard]) {
			continue
		}
		for _, key := range exactHiveMemPermissionKeys() {
			if isGeneratedHiveAllow(permission[key]) {
				delete(permission, key)
			}
		}
		return
	}
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
