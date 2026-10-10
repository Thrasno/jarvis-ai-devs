package applyprogress

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func recordStep(command string, exitCode int, summary string) *TaskStep {
	return &TaskStep{Command: command, ExitCode: exitCode, Summary: summary}
}

func recordEntry(id, task string, completes bool, kind EvidenceKind, command string, exitCode int, outcome Outcome, summary string, files []string) EvidenceEntry {
	completed := []string{}
	if completes {
		completed = []string{task}
	}
	return EvidenceEntry{EntryID: id, TaskIDs: []string{task}, CompletesTaskIDs: completed, Kind: kind, Summary: summary, Command: command, ExitCode: exitCode, Outcome: outcome, Files: files}
}

func TestExpandTaskRecordsProducesCanonicalEntries(t *testing.T) {
	files := []string{"a.go", "a_test.go"}
	noCompletion := false
	longID := strings.Repeat("t", 60)
	longStem := longID[:42] + "-" + digest([]byte(longID))[:8]
	for name, test := range map[string]struct {
		records []TaskRecord
		want    []EvidenceEntry
	}{
		"strict full record": {
			records: []TaskRecord{{TaskID: "1.2", Files: files,
				Red:          recordStep("go test ./x -run TestA", 1, "fails: missing Foo"),
				Green:        recordStep("go test ./x -run TestA", 0, "passes"),
				Triangulate:  recordStep("go test ./x -run TestA/edge", 0, "edge passes"),
				Refactor:     recordStep("go test ./x", 0, "still green"),
				Verification: recordStep("go test ./...", 0, "suite green"),
			}},
			want: []EvidenceEntry{
				recordEntry("1.2-red", "1.2", false, EvidenceRed, "go test ./x -run TestA", 1, OutcomeFail, "fails: missing Foo", files),
				recordEntry("1.2-green", "1.2", false, EvidenceGreen, "go test ./x -run TestA", 0, OutcomePass, "passes", files),
				recordEntry("1.2-triangulate", "1.2", false, EvidenceTriangulate, "go test ./x -run TestA/edge", 0, OutcomePass, "edge passes", files),
				recordEntry("1.2-refactor", "1.2", false, EvidenceRefactor, "go test ./x", 0, OutcomePass, "still green", files),
				recordEntry("1.2-verification", "1.2", true, EvidenceVerification, "go test ./...", 0, OutcomePass, "suite green", files),
			},
		},
		"skipped triangulation": {
			records: []TaskRecord{{TaskID: "2", Files: files,
				Red:         recordStep("go test ./y", 2, "fails"),
				Green:       recordStep("go test ./y", 0, "passes"),
				Triangulate: &TaskStep{SkipReason: "structural: single branch"},
			}},
			want: []EvidenceEntry{
				recordEntry("2-red", "2", false, EvidenceRed, "go test ./y", 2, OutcomeFail, "fails", files),
				recordEntry("2-green", "2", true, EvidenceGreen, "go test ./y", 0, OutcomePass, "passes", files),
				recordEntry("2-triangulate", "2", false, EvidenceTriangulate, "", 0, OutcomeNotRun, "structural: single branch", files),
			},
		},
		"standard verification only without files": {
			records: []TaskRecord{{TaskID: "3", Verification: recordStep("", 0, "documentation only; nothing runnable")}},
			want:    []EvidenceEntry{recordEntry("3-verification", "3", true, EvidenceVerification, "", 0, OutcomePass, "documentation only; nothing runnable", []string{})},
		},
		"non-completing record": {
			records: []TaskRecord{{TaskID: "4", Completes: &noCompletion, Files: files, Red: recordStep("go test", 1, "fails"), Green: recordStep("go test", 0, "passes")}},
			want: []EvidenceEntry{
				recordEntry("4-red", "4", false, EvidenceRed, "go test", 1, OutcomeFail, "fails", files),
				recordEntry("4-green", "4", false, EvidenceGreen, "go test", 0, OutcomePass, "passes", files),
			},
		},
		"records keep their order": {
			records: []TaskRecord{
				{TaskID: "b", Verification: recordStep("make check", 0, "ok")},
				{TaskID: "a", Verification: recordStep("make check", 0, "ok")},
			},
			want: []EvidenceEntry{
				recordEntry("b-verification", "b", true, EvidenceVerification, "make check", 0, OutcomePass, "ok", []string{}),
				recordEntry("a-verification", "a", true, EvidenceVerification, "make check", 0, OutcomePass, "ok", []string{}),
			},
		},
		"long task IDs derive bounded entry IDs": {
			records: []TaskRecord{{TaskID: longID, Red: recordStep("go test", 1, "fails"), Verification: recordStep("go test", 0, "passes")}},
			want: []EvidenceEntry{
				recordEntry(longStem+"-red", longID, false, EvidenceRed, "go test", 1, OutcomeFail, "fails", []string{}),
				recordEntry(longStem+"-verification", longID, true, EvidenceVerification, "go test", 0, OutcomePass, "passes", []string{}),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ExpandTaskRecords(test.records)
			if err != nil {
				t.Fatalf("ExpandTaskRecords() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ExpandTaskRecords() =\n%#v\nwant\n%#v", got, test.want)
			}
			for _, entry := range got {
				if err := validateEvidenceEntry(entry); err != nil || len(entry.EntryID) > 64 {
					t.Fatalf("expanded entry %q is not canonical: %v", entry.EntryID, err)
				}
			}
			again, err := ExpandTaskRecords(test.records)
			if err != nil || !reflect.DeepEqual(again, got) || mustStream(t, again) != mustStream(t, got) {
				t.Fatalf("identical records expanded differently: %#v, %v", again, err)
			}
		})
	}
}

func TestExpandTaskRecordsStreamValidatesAgainstTasks(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "one"}, {ID: "2", Text: "two"}}
	entries, err := ExpandTaskRecords([]TaskRecord{
		{TaskID: "1", Red: recordStep("go test", 1, "fails"), Green: recordStep("go test", 0, "passes")},
		{TaskID: "2", Verification: recordStep("go vet ./...", 0, "clean")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateFutureStream(tasks, entries, nil); err != nil {
		t.Fatalf("expanded stream rejected by existing validators: %v", err)
	}
}

// TestExpandTaskRecordsCompletesOnLastRunPass attaches completion to the last run
// step, so a skipped triangulation after GREEN does not hide a passing completion.
func TestExpandTaskRecordsCompletesOnLastRunPass(t *testing.T) {
	entries, err := ExpandTaskRecords([]TaskRecord{{
		TaskID:      "1",
		Red:         recordStep("go test", 1, "fails"),
		Green:       recordStep("go test", 0, "passes"),
		Triangulate: &TaskStep{SkipReason: "single branch"},
	}})
	if err != nil || len(entries) != 3 {
		t.Fatalf("ExpandTaskRecords() = %#v, %v", entries, err)
	}
	if len(entries[1].CompletesTaskIDs) != 1 || len(entries[2].CompletesTaskIDs) != 0 {
		t.Fatalf("completion = green %v, triangulate %v; want it on green", entries[1].CompletesTaskIDs, entries[2].CompletesTaskIDs)
	}
}

func TestExpandTaskRecordsRejectsInvalidRecords(t *testing.T) {
	longID := strings.Repeat("t", 60)
	collidingID := longID[:42] + "-" + digest([]byte(longID))[:8]
	ok := recordStep("go test", 0, "passes")
	for name, test := range map[string]struct {
		records []TaskRecord
		detail  string
	}{
		"no records":                      {records: []TaskRecord{}, detail: "task_records"},
		"empty record":                    {records: []TaskRecord{{TaskID: "1"}}, detail: "task_records[0]"},
		"invalid task ID":                 {records: []TaskRecord{{TaskID: "bad id", Verification: ok}}, detail: "task_records[0].task_id"},
		"missing task ID":                 {records: []TaskRecord{{Verification: ok}}, detail: "task_records[0].task_id"},
		"duplicate task IDs":              {records: []TaskRecord{{TaskID: "1", Verification: ok}, {TaskID: "1", Red: ok}}, detail: "task_records[1].task_id"},
		"empty step":                      {records: []TaskRecord{{TaskID: "1", Green: &TaskStep{}}}, detail: "task_records[0].green"},
		"skip reason outside triangulate": {records: []TaskRecord{{TaskID: "1", Refactor: &TaskStep{SkipReason: "nothing to refactor"}}}, detail: "task_records[0].refactor"},
		"skip reason with a command":      {records: []TaskRecord{{TaskID: "1", Triangulate: &TaskStep{SkipReason: "single branch", Command: "go test"}}}, detail: "task_records[0].triangulate"},
		"skip reason with an exit code":   {records: []TaskRecord{{TaskID: "1", Triangulate: &TaskStep{SkipReason: "single branch", ExitCode: 1}}}, detail: "task_records[0].triangulate"},
		"derived entry ID collision":      {records: []TaskRecord{{TaskID: longID, Verification: ok}, {TaskID: collidingID, Verification: ok}}, detail: "task_records[1].entry_id"},
		"completion ending in RED":        {records: []TaskRecord{{TaskID: "1", Red: recordStep("go test", 1, "fails")}}, detail: "task_records[0].completes"},
		"completion ending in failure":    {records: []TaskRecord{{TaskID: "1", Red: recordStep("go test", 1, "fails"), Green: recordStep("go test", 2, "still fails")}}, detail: "task_records[0].completes"},
	} {
		t.Run(name, func(t *testing.T) {
			entries, err := ExpandTaskRecords(test.records)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != CodeInvalidPlan || validation.Detail != test.detail || entries != nil {
				t.Fatalf("ExpandTaskRecords() = %#v, %v; want %s %q", entries, err, CodeInvalidPlan, test.detail)
			}
			if !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("error %v does not unwrap to ErrInvalidValue", err)
			}
		})
	}
}

func mustStream(t *testing.T, entries []EvidenceEntry) string {
	t.Helper()
	stream, err := StreamSHA256(entries)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}
