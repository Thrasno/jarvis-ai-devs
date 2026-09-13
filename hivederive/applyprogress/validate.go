package applyprogress

import (
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type ValidationCode string

const (
	CodeCorruptSnapshot      ValidationCode = "corrupt_snapshot"
	CodeCorruptBatch         ValidationCode = "corrupt_batch"
	CodeTaskManifestMismatch ValidationCode = "task_manifest_mismatch"
	CodeDuplicateEvidence    ValidationCode = "duplicate_evidence"
	CodeMissingBatch         ValidationCode = "missing_batch"
	CodeInvalidCoverage      ValidationCode = "invalid_coverage"
	CodeLegacyAmbiguous      ValidationCode = "legacy_ambiguous"
)

type ValidationError struct{ Code ValidationCode }

func (e *ValidationError) Error() string { return string(e.Code) }
func invalid(code ValidationCode) error  { return &ValidationError{Code: code} }

// Task is a current explicit task or a legacy task identified by Path.
type Task struct {
	ID   string
	Path string
	Text string
}

// TaskManifest normalizes task text and returns its deterministic ordered digest.
func TaskManifest(tasks []Task) ([]Task, string, error) {
	if len(tasks) == 0 {
		return nil, "", invalid(CodeLegacyAmbiguous)
	}
	out := make([]Task, len(tasks))
	seen := map[string]bool{}
	for i, task := range tasks {
		text, err := NormalizeTaskText(task.Text)
		if err != nil {
			return nil, "", err
		}
		id := task.ID
		if id == "" {
			id, err = LegacyTaskID(task.Path, text)
			if err != nil {
				return nil, "", err
			}
		} else if !validID(id) {
			return nil, "", invalid(CodeLegacyAmbiguous)
		}
		if seen[id] {
			return nil, "", invalid(CodeLegacyAmbiguous)
		}
		seen[id] = true
		out[i] = Task{ID: id, Path: task.Path, Text: text}
	}
	manifest := make([]struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}, len(out))
	for i, task := range out {
		manifest[i] = struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}{task.ID, task.Text}
	}
	data, err := canonicalJSON(manifest)
	if err != nil {
		return nil, "", err
	}
	return out, digest(data), nil
}

func NormalizeTaskText(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", invalid(CodeLegacyAmbiguous)
	}
	text = strings.Join(strings.Fields(norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"))), " ")
	if text == "" {
		return "", invalid(CodeLegacyAmbiguous)
	}
	return text, nil
}

func LegacyTaskID(path, text string) (string, error) {
	if !validID(path) {
		return "", invalid(CodeLegacyAmbiguous)
	}
	text, err := NormalizeTaskText(text)
	if err != nil {
		return "", err
	}
	return "legacy-" + digest([]byte(path + "\n" + text))[:32], nil
}

// ValidateEvidenceCoverage verifies snapshot coverage against its immutable evidence.
// It intentionally does not require a task manifest.
func ValidateEvidenceCoverage(snapshot Snapshot, batches map[string]Batch) error {
	if snapshot.Status == StatusComplete && (len(snapshot.Coverage) == 0 || len(snapshot.Batches) == 0) {
		return invalid(CodeInvalidCoverage)
	}
	completed := make(map[string]Coverage)
	for _, ref := range snapshot.Batches {
		batch, ok := batches[ref.BatchID]
		if !ok {
			return invalid(CodeMissingBatch)
		}
		entries := map[string]bool{}
		for _, entry := range batch.Entries {
			if entries[entry.EntryID] {
				return invalid(CodeDuplicateEvidence)
			}
			entries[entry.EntryID] = true
			for _, id := range entry.CompletesTaskIDs {
				if _, exists := completed[id]; exists {
					return invalid(CodeDuplicateEvidence)
				}
				completed[id] = Coverage{TaskID: id, BatchID: batch.BatchID, EntryID: entry.EntryID}
			}
		}
	}
	covered := map[string]bool{}
	for _, coverage := range snapshot.Coverage {
		if covered[coverage.TaskID] || completed[coverage.TaskID] != coverage {
			return invalid(CodeInvalidCoverage)
		}
		covered[coverage.TaskID] = true
	}
	if len(covered) != len(completed) {
		return invalid(CodeInvalidCoverage)
	}
	return nil
}

// ValidateProgress accepts only one canonical snapshot and its referenced batches.
// Batches not referenced by the snapshot are deliberately non-authoritative.
func ValidateProgress(snapshotData []byte, tasks []Task, batches map[string][]byte) error {
	snapshot, err := DecodeCanonicalSnapshot(snapshotData)
	if err != nil {
		return invalid(CodeCorruptSnapshot)
	}
	normalized, manifest, err := TaskManifest(tasks)
	if err != nil {
		return err
	}
	if snapshot.TaskManifestSHA256 != manifest {
		return invalid(CodeTaskManifestMismatch)
	}
	known := make(map[string]bool, len(normalized))
	for _, task := range normalized {
		known[task.ID] = true
	}
	seenBatches, completed := map[string]bool{}, map[string]Coverage{}
	for _, ref := range snapshot.Batches {
		if seenBatches[ref.BatchID] {
			return invalid(CodeDuplicateEvidence)
		}
		seenBatches[ref.BatchID] = true
		raw, ok := batches[ref.BatchID]
		if !ok {
			return invalid(CodeMissingBatch)
		}
		batch, err := DecodeCanonicalBatch(raw)
		if err != nil || batch.BatchID != ref.BatchID || batch.SHA256 != ref.SHA256 || batch.Project != snapshot.Project || batch.Change != snapshot.Change {
			return invalid(CodeCorruptBatch)
		}
		entries := map[string]bool{}
		for _, entry := range batch.Entries {
			if entries[entry.EntryID] {
				return invalid(CodeDuplicateEvidence)
			}
			entries[entry.EntryID] = true
			attributed := map[string]bool{}
			for _, id := range entry.TaskIDs {
				if !known[id] || attributed[id] {
					return invalid(CodeInvalidCoverage)
				}
				attributed[id] = true
			}
			for _, id := range entry.CompletesTaskIDs {
				if !known[id] || !attributed[id] {
					return invalid(CodeInvalidCoverage)
				}
				if _, exists := completed[id]; exists {
					return invalid(CodeDuplicateEvidence)
				}
				completed[id] = Coverage{TaskID: id, BatchID: batch.BatchID, EntryID: entry.EntryID}
			}
		}
	}
	coverage := make([]Coverage, 0, len(completed))
	for _, task := range normalized {
		if item, ok := completed[task.ID]; ok {
			coverage = append(coverage, item)
		}
	}
	if !slices.Equal(snapshot.Coverage, coverage) {
		return invalid(CodeInvalidCoverage)
	}
	full := len(completed) == len(normalized)
	if (snapshot.Status == StatusComplete) != full {
		return invalid(CodeInvalidCoverage)
	}
	return nil
}
