package applyprogress

import (
	"crypto/sha256"
	"encoding/hex"
)

// LegacySourceSHA256 returns the exact-byte identity of the authoritative
// legacy apply-progress artifact consumed by an imported-evidence migration.
func LegacySourceSHA256(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

// ContainsImportedEvidence reports whether an appended batch asserts legacy
// completion evidence. Callers must authorize that assertion before writing.
func ContainsImportedEvidence(batches []Batch) bool {
	for _, batch := range batches {
		for _, entry := range batch.Entries {
			if entry.Kind == EvidenceImported {
				return true
			}
		}
	}
	return false
}

// LegacyProgress is the conservative, already-parsed view of a cumulative v1 artifact.
type LegacyProgress struct {
	Tasks     []Task
	Completed []string
}

type LegacyConversion struct {
	Tasks              []Task
	Completed          []string
	TaskManifestSHA256 string
	// EvidenceKind lets upgrade callers require an explicit imported-evidence record for legacy completions.
	EvidenceKind EvidenceKind
}

// ConvertLegacy normalizes legacy tasks and refuses ambiguous completion markers.
// LegacyTaskManifest derives the stable legacy identities for checkbox tasks parsed
// from tasks.md. It lets readers validate an upgraded legacy snapshot without
// reimplementing a second task parser or identity heuristic.
func LegacyTaskManifest(tasks []Task) ([]Task, string, error) {
	legacy := LegacyProgress{Tasks: make([]Task, len(tasks))}
	for i, task := range tasks {
		path := task.Path
		if path == "" {
			path = task.ID
		}
		legacy.Tasks[i] = Task{Path: path, Text: task.Text}
	}
	converted, err := ConvertLegacy(legacy)
	if err != nil {
		return nil, "", err
	}
	return converted.Tasks, converted.TaskManifestSHA256, nil
}

func ConvertLegacy(progress LegacyProgress) (LegacyConversion, error) {
	tasks := make([]Task, len(progress.Tasks))
	manifestItems := make([]struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}, len(progress.Tasks))
	byPath, identities, explicitIDs := map[string]string{}, map[string]bool{}, map[string]bool{}
	for i, task := range progress.Tasks {
		if !validID(task.Path) || byPath[task.Path] != "" {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous, "legacy task path")
		}
		text, err := NormalizeTaskText(task.Text)
		if err != nil {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous, "legacy task manifest")
		}
		id, err := LegacyTaskID(task.Path, text)
		if err != nil {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous, "legacy task manifest")
		}
		tasks[i] = Task{ID: id, Path: task.Path, Text: text}
		manifestItems[i] = struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}{ID: id, Text: text}
		byPath[task.Path], identities[id] = id, true
		if task.ID != "" && task.ID != task.Path {
			explicitIDs[task.ID] = true
		}
	}
	data, err := canonicalJSON(manifestItems)
	if err != nil {
		return LegacyConversion{}, err
	}
	manifest := digest(data)
	completed, used := make([]string, len(progress.Completed)), map[string]bool{}
	for i, marker := range progress.Completed {
		id := marker
		if pathID := byPath[marker]; pathID != "" {
			if isAmbiguousLegacyMarker(marker, pathID, identities) || explicitIDs[marker] {
				return LegacyConversion{}, invalid(CodeLegacyAmbiguous, "legacy completion marker")
			}
			id = pathID
		}
		if invalidLegacyCompletion(id, identities, used) {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous, "legacy completion")
		}
		used[id], completed[i] = true, id
	}
	return LegacyConversion{Tasks: tasks, Completed: completed, TaskManifestSHA256: manifest, EvidenceKind: EvidenceImported}, nil
}

func isAmbiguousLegacyMarker(marker, pathID string, identities map[string]bool) bool {
	return identities[marker] && marker != pathID
}

func invalidLegacyCompletion(id string, identities, used map[string]bool) bool {
	return !identities[id] || used[id]
}
