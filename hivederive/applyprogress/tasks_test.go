package applyprogress

import (
	"errors"
	"testing"
)

func TestParseTasksMarkdownProvidesOneCanonicalTaskView(t *testing.T) {
	parsed, err := ParseTasksMarkdown("# Tasks\n\n- [x] 1.1 first task\n- [ ] 1.2 second task\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tasks) != 2 || parsed.Tasks[0].ID != "1.1" || parsed.Tasks[0].Text != "first task" || parsed.Completed != 1 || parsed.AllDone {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseTasksMarkdownRejectsAmbiguousCheckboxRows(t *testing.T) {
	_, err := ParseTasksMarkdown("- [x] 1.1 first\n- [z] 1.2 malformed\n")
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeInvalidTask || validation.Detail != "line 2" {
		t.Fatalf("ParseTasksMarkdown() error = %#v, want invalid_task at line 2", err)
	}
}

func TestParseTasksMarkdownAcceptsOnlyRepositoryImplementationIDs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "numeric", content: "- [ ] 1 implementation\n"},
		{name: "dotted numeric", content: "- [x] 1.2.3 implementation\n"},
		{name: "parent prose outside parent section", content: "- [ ] Start implementation\n", wantErr: true},
		{name: "generic protocol identifier", content: "- [ ] TASK-1 implementation\n", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTasksMarkdown(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTasksMarkdown(%q) error = %v, wantErr %t", tt.content, err, tt.wantErr)
			}
		})
	}

	if _, _, err := TaskManifest([]Task{{ID: "Start", Text: "generic protocol task"}}); err != nil {
		t.Fatalf("TaskManifest() rejected unchanged generic protocol ID: %v", err)
	}
}

func TestParseTasksMarkdownAcceptsEstablishedLetterPrefixedIDsAndSkipsAllParentRows(t *testing.T) {
	parsed, err := ParseTasksMarkdown(`# Tasks
- [ ] C2.1 component task
- [x] R1 release task
- [ ] 2.3 numeric task

## Parent Actions
- [z] malformed parent action
- [ ] TASK-1 parent prose
`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := parsed.CompletedIDs, []string{"R1"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("completed IDs = %#v, want %#v", got, want)
	}
	if len(parsed.Tasks) != 3 || parsed.Tasks[0].ID != "C2.1" || parsed.Tasks[1].ID != "R1" || parsed.Tasks[2].ID != "2.3" {
		t.Fatalf("tasks = %#v", parsed.Tasks)
	}
}

func TestParseTasksMarkdownAcceptsLowercaseNamespaceAndHonorsParentHeadingDepth(t *testing.T) {
	parsed, err := ParseTasksMarkdown(`# Tasks
- [ ] 7a.1 established lowercase namespace

## Parent Actions
- [z] malformed parent action
### Nested parent details
- [ ] 8.1 nested parent action
## Implementation
- [x] 8.1 implementation task
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tasks) != 2 || parsed.Tasks[0].ID != "7a.1" || parsed.Tasks[1].ID != "8.1" || parsed.Completed != 1 {
		t.Fatalf("parsed = %#v, want lowercase ID plus task after parent section", parsed)
	}
}

func TestParseLegacyTasksMarkdownAcceptsHistoricalPhaseCheckboxesAndExcludesParentActions(t *testing.T) {
	parsed, err := ParseLegacyTasksMarkdown(`# Historical Tasks
## 1. Compatibility
- [x] RED: decoder rejects historical corruption
- [x] GREEN: decoder accepts canonical snapshots
- [ ] TRIANGULATE: exercise an alternate digest
- [ ] REFACTOR: clarify compatibility path

## Parent Actions
- [x] RED: this is parent workflow prose
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tasks) != 4 || parsed.Completed != 2 || parsed.AllDone {
		t.Fatalf("parsed = %#v", parsed)
	}
	for _, task := range parsed.Tasks {
		if task.ID == "" || task.Path == "" || task.Text == "" {
			t.Fatalf("legacy task lacks deterministic identity: %#v", task)
		}
	}
	converted, err := ConvertLegacy(LegacyProgress{Tasks: parsed.Tasks, Completed: parsed.CompletedIDs})
	if err != nil || len(converted.Tasks) != 4 || len(converted.Completed) != 2 {
		t.Fatalf("ConvertLegacy() = %#v, %v", converted, err)
	}
}

func TestParseTasksMarkdownIgnoresIssue653ParentChecklist(t *testing.T) {
	const archivedTasks = `# Tasks: Bounded, Verifiable Apply Progress

## 1A. Protocol Models

- [x] 1A.1 implement models
- [x] 1A.2 implement validation

## Parent Actions After Implementation

- [ ] Start or reuse bounded review after each selected work unit
`

	parsed, err := ParseTasksMarkdown(archivedTasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tasks) != 2 || parsed.Completed != 2 || !parsed.AllDone {
		t.Fatalf("parsed = %#v, want only two completed implementation tasks", parsed)
	}
	for _, task := range parsed.Tasks {
		if task.ID == "Start" {
			t.Fatalf("parsed parent checklist prose as a task: %#v", parsed)
		}
	}
}
