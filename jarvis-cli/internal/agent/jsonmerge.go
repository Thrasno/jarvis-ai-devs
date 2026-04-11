package agent

import (
	"encoding/json"
	"fmt"
)

// MergeJSON performs a deep merge of patch into base.
//
// Rules:
//   - For objects (map[string]any): patch keys are merged into base recursively.
//     The special key "hive" (Jarvis-managed) is always overwritten by patch.
//   - For arrays ([]any): patch items are appended if not already present.
//     Presence is determined by matching on "name" or "type" identity key.
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
// For the "hive" key specifically (Jarvis-owned), src always wins.
func deepMerge(dst, src map[string]any) map[string]any {
	result := make(map[string]any, len(dst))

	// Copy all dst keys first
	for k, v := range dst {
		result[k] = v
	}

	// Apply src keys
	for k, srcVal := range src {
		// "hive" is always owned by Jarvis — overwrite unconditionally
		if k == "hive" {
			result[k] = srcVal
			continue
		}

		dstVal, exists := result[k]
		if !exists {
			// New key from patch — add it
			result[k] = srcVal
			continue
		}

		// Both exist — merge by type
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)

		if srcIsMap && dstIsMap {
			result[k] = deepMerge(dstMap, srcMap)
			continue
		}

		srcArr, srcIsArr := srcVal.([]any)
		dstArr, dstIsArr := dstVal.([]any)

		if srcIsArr && dstIsArr {
			result[k] = mergeArrays(dstArr, srcArr)
			continue
		}

		// Scalar: patch wins
		result[k] = srcVal
	}

	return result
}

// mergeArrays appends items from src that are not already present in dst.
// Identity is determined by matching "name" or "type" fields (for objects),
// or by direct equality (for scalars).
func mergeArrays(dst, src []any) []any {
	result := make([]any, len(dst))
	copy(result, dst)

	for _, srcItem := range src {
		if !arrayContains(result, srcItem) {
			result = append(result, srcItem)
		}
	}

	return result
}

// arrayContains returns true if arr contains an item matching item.
// For map items, matching is done by "name" or "type" field.
func arrayContains(arr []any, item any) bool {
	itemMap, itemIsMap := item.(map[string]any)

	for _, existing := range arr {
		if itemIsMap {
			existingMap, existingIsMap := existing.(map[string]any)
			if !existingIsMap {
				continue
			}
			// Match by "name" field first, then "type"
			for _, key := range []string{"name", "type"} {
				itemVal, iOk := itemMap[key]
				existingVal, eOk := existingMap[key]
				if iOk && eOk && itemVal == existingVal {
					return true
				}
			}
		} else {
			// Scalar comparison
			if existing == item {
				return true
			}
		}
	}

	return false
}
