package applyprogress

import (
	"errors"
	"testing"
)

func operatorEntry(id, task string) EvidenceEntry {
	return EvidenceEntry{EntryID: id, TaskIDs: []string{task}, CompletesTaskIDs: []string{task}, Kind: EvidenceOperator, Summary: "done, continue", Outcome: OutcomePass, Files: []string{}}
}

// TestOperatorEvidenceCompletesOnlyOperatorTasks keeps the developer's chat
// acknowledgement from completing agent work: operator evidence may only attribute
// tasks whose text carries the [operator] tag.
func TestOperatorEvidenceCompletesOnlyOperatorTasks(t *testing.T) {
	tasks := []Task{{ID: "1", Text: "[operator] Upload the function and run cases A and B"}, {ID: "2", Text: "Write the parser"}}
	agent := EvidenceEntry{EntryID: "2-verification", TaskIDs: []string{"2"}, CompletesTaskIDs: []string{"2"}, Kind: EvidenceVerification, Summary: "ok", Command: "go test", Outcome: OutcomePass, Files: []string{}}

	if err := ValidateFutureStream(tasks, []EvidenceEntry{operatorEntry("1-operator", "1"), agent}, nil); err != nil {
		t.Fatalf("operator evidence on an [operator] task rejected: %v", err)
	}
	err := ValidateFutureStream(tasks, []EvidenceEntry{operatorEntry("1-operator", "1"), operatorEntry("2-operator", "2")}, nil)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeInvalidEvidence || validation.Detail != "2-operator" {
		t.Fatalf("operator evidence on an agent task = %v; want %s 2-operator", err, CodeInvalidEvidence)
	}
}

// TestOperatorEvidenceShapeIsAttestationOnly rejects operator entries that claim a
// command run, a non-zero exit code, or a non-pass outcome on the explicit entries path.
func TestOperatorEvidenceShapeIsAttestationOnly(t *testing.T) {
	for name, mutate := range map[string]func(*EvidenceEntry){
		"command":   func(e *EvidenceEntry) { e.Command = "go test" },
		"exit code": func(e *EvidenceEntry) { e.ExitCode = 1 },
		"failure":   func(e *EvidenceEntry) { e.Outcome = OutcomeFail },
		"not run":   func(e *EvidenceEntry) { e.Outcome = OutcomeNotRun },
	} {
		t.Run(name, func(t *testing.T) {
			entry := operatorEntry("1-operator", "1")
			mutate(&entry)
			if err := validateEvidenceEntry(entry); !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("validateEvidenceEntry(%+v) = %v; want ErrInvalidValue", entry, err)
			}
		})
	}
	if err := validateEvidenceEntry(operatorEntry("1-operator", "1")); err != nil {
		t.Fatalf("valid operator entry rejected: %v", err)
	}
}
