package applyprogress

import "strconv"

// TaskRecord is the compact per-task evidence form. ExpandTaskRecords turns it
// into canonical evidence entries, so planning, batching, continuation, and
// replay operate on exactly the stream an explicit entry list would produce.
type TaskRecord struct {
	TaskID string   `json:"task_id"`
	Files  []string `json:"files,omitempty"`
	// Completes defaults to true: the last expanded entry completes the task.
	Completes    *bool     `json:"completes,omitempty"`
	Red          *TaskStep `json:"red,omitempty"`
	Green        *TaskStep `json:"green,omitempty"`
	Triangulate  *TaskStep `json:"triangulate,omitempty"`
	Refactor     *TaskStep `json:"refactor,omitempty"`
	Verification *TaskStep `json:"verification,omitempty"`
}

// TaskStep is one observed step of a TaskRecord. The outcome derives from the
// exit code; SkipReason records a triangulation that was deliberately not run.
type TaskStep struct {
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Summary    string `json:"summary,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
}

// taskRecordStemLimit leaves room for the longest "-<step>" suffix within the
// protocol ID limit of 64 characters.
const taskRecordStemLimit = 64 - len("-verification")

// ExpandTaskRecords deterministically expands compact records, in order, into
// canonical evidence entries with steps ordered red, green, triangulate,
// refactor, verification. Entry IDs derive from the task ID and step name.
// Semantic stream rules stay with the existing stream validators.
func ExpandTaskRecords(records []TaskRecord) ([]EvidenceEntry, error) {
	if len(records) == 0 {
		return nil, invalid(CodeInvalidPlan, "task_records")
	}
	entries := []EvidenceEntry{}
	seenTasks, seenEntries := map[string]bool{}, map[string]bool{}
	for i, record := range records {
		field := "task_records[" + strconv.Itoa(i) + "]"
		if !validID(record.TaskID) || seenTasks[record.TaskID] {
			return nil, invalid(CodeInvalidPlan, field+".task_id")
		}
		seenTasks[record.TaskID] = true
		steps := []struct {
			name string
			kind EvidenceKind
			step *TaskStep
		}{
			{"red", EvidenceRed, record.Red},
			{"green", EvidenceGreen, record.Green},
			{"triangulate", EvidenceTriangulate, record.Triangulate},
			{"refactor", EvidenceRefactor, record.Refactor},
			{"verification", EvidenceVerification, record.Verification},
		}
		stem := taskRecordStem(record.TaskID)
		first := len(entries)
		for _, step := range steps {
			if step.step == nil {
				continue
			}
			entry, ok := expandTaskStep(step.kind, *step.step)
			if !ok {
				return nil, invalid(CodeInvalidPlan, field+"."+step.name)
			}
			entry.EntryID = stem + "-" + step.name
			if seenEntries[entry.EntryID] {
				return nil, invalid(CodeInvalidPlan, field+".entry_id")
			}
			seenEntries[entry.EntryID] = true
			entry.TaskIDs, entry.CompletesTaskIDs, entry.Files = []string{record.TaskID}, []string{}, append([]string{}, record.Files...)
			entries = append(entries, entry)
		}
		if len(entries) == first {
			return nil, invalid(CodeInvalidPlan, field)
		}
		if record.Completes == nil || *record.Completes {
			// Completion rides on the last step that actually ran, and that step must
			// pass: a record cannot complete a task on RED or failing evidence.
			last := len(entries) - 1
			for last > first && entries[last].Outcome == OutcomeNotRun {
				last--
			}
			if entries[last].Outcome != OutcomePass {
				return nil, invalid(CodeInvalidPlan, field+".completes")
			}
			entries[last].CompletesTaskIDs = []string{record.TaskID}
		}
	}
	return entries, nil
}

func expandTaskStep(kind EvidenceKind, step TaskStep) (EvidenceEntry, bool) {
	if step.SkipReason != "" {
		if kind != EvidenceTriangulate || step.Command != "" || step.ExitCode != 0 || step.Summary != "" {
			return EvidenceEntry{}, false
		}
		return EvidenceEntry{Kind: kind, Summary: step.SkipReason, Outcome: OutcomeNotRun}, true
	}
	if step.Command == "" && step.Summary == "" {
		return EvidenceEntry{}, false
	}
	outcome := OutcomePass
	if step.ExitCode != 0 {
		outcome = OutcomeFail
	}
	return EvidenceEntry{Kind: kind, Summary: step.Summary, Command: step.Command, ExitCode: step.ExitCode, Outcome: outcome}, true
}

// taskRecordStem keeps short task IDs readable and bounds long ones with a
// digest suffix. A crafted short ID can still equal a bounded stem, so callers
// must reject colliding derived entry IDs.
func taskRecordStem(taskID string) string {
	if len(taskID) <= taskRecordStemLimit {
		return taskID
	}
	return taskID[:taskRecordStemLimit-9] + "-" + digest([]byte(taskID))[:8]
}
