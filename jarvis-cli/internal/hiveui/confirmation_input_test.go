package hiveui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// confirmationWizard describes one typed-confirmation destructive flow so the
// normalization, key handling and feedback contracts can be asserted uniformly
// across every wizard that asks the operator to type a phrase.
type confirmationWizard struct {
	name string
	// at returns a model parked on the confirmation step with an empty input.
	at func() Model
	// phrase is the exact confirmation phrase the wizard expects.
	phrase string
	// wrong is a phrase that must never be accepted.
	wrong string
}

func confirmationWizards() []confirmationWizard {
	return []confirmationWizard{
		{
			name:   "memory guard delete",
			at:     memoryGuardAtConfirmation,
			phrase: "DELETE memory 7",
			wrong:  "DELETE memory 8",
		},
		{
			name:   "project archive",
			at:     projectArchiveAtConfirmation,
			phrase: "ARCHIVE project alpha",
			wrong:  "ARCHIVE project beta",
		},
		{
			name:   "project purge",
			at:     projectPurgeAtConfirmation,
			phrase: "PURGE project alpha",
			wrong:  "PURGE project beta",
		},
		{
			name:   "project merge",
			at:     projectMergeAtConfirmation,
			phrase: "MERGE project alpha INTO beta",
			wrong:  "MERGE project beta INTO alpha",
		},
		{
			name:   "batch project merge",
			at:     batchMergeAtConfirmation,
			phrase: "MERGE projects INTO beta",
			wrong:  "MERGE projects INTO alpha",
		},
	}
}

func memoryGuardAtConfirmation() Model {
	m := NewModelWithSnapshotAndGuardExecutor(guardedMemorySnapshot(), &fakeGuardExecutor{})
	m = openGuardedMemoryDelete(m)
	m = sendText(m, "backup-1")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "stale cleanup")
	m = sendKey(m, tea.KeyEnter)
	return m
}

func projectArchiveAtConfirmation() Model {
	m := NewModelWithSnapshotAndProjectArchiveExecutor(projectArchiveSnapshot(), &fakeProjectArchiveExecutor{})
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'a')
	m = sendText(m, "backup-archive")
	m = sendKey(m, tea.KeyEnter)
	return m
}

func projectPurgeAtConfirmation() Model {
	m := NewModelWithSnapshotAndProjectDeleteExecutor(projectPurgeSnapshot(), &fakeProjectDeleteExecutor{})
	m = activatePurgeFromDashboard(m)
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "backup-purge")
	m = sendKey(m, tea.KeyEnter)
	return m
}

func projectMergeAtConfirmation() Model {
	m := openProjectMerge(&fakeProjectMergeExecutor{})
	m = sendRune(m, 'm')
	m = sendText(m, "beta")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "backup-merge")
	m = sendKey(m, tea.KeyEnter)
	return m
}

func batchMergeAtConfirmation() Model {
	return openBatchProjectMergeAtConfirm(&fakeProjectMergeBatchExecutor{})
}

// submitsConfirmation reports whether pressing enter dispatched the guarded
// operation. Every wizard returns a command only when the confirmation passed.
func submitsConfirmation(m Model) bool {
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return cmd != nil
}

func TestConfirmationAcceptsNormalizedWhitespaceForEveryWizard(t *testing.T) {
	variants := map[string]func(string) string{
		"leading space":        func(phrase string) string { return " " + phrase },
		"trailing space":       func(phrase string) string { return phrase + " " },
		"doubled inner spaces": func(phrase string) string { return strings.ReplaceAll(phrase, " ", "  ") },
		"padding and tripled inner spaces": func(phrase string) string {
			return "  " + strings.ReplaceAll(phrase, " ", "   ") + "  "
		},
	}

	for _, wizard := range confirmationWizards() {
		for variant, transform := range variants {
			t.Run(wizard.name+"/"+variant, func(t *testing.T) {
				m := sendText(wizard.at(), transform(wizard.phrase))
				if !submitsConfirmation(m) {
					t.Fatalf("%s rejected normalized confirmation %q", wizard.name, transform(wizard.phrase))
				}
			})
		}
	}
}

// Casing stays part of the deliberate friction: only whitespace is forgiven.
func TestConfirmationRejectsWrongCasingForEveryWizard(t *testing.T) {
	variants := map[string]func(string) string{
		"lowercase":             strings.ToLower,
		"uppercase":             strings.ToUpper,
		"padded lowercase":      func(phrase string) string { return "  " + strings.ToLower(phrase) + "  " },
		"lowercased first rune": func(phrase string) string { return strings.ToLower(phrase[:1]) + phrase[1:] },
	}

	for _, wizard := range confirmationWizards() {
		for variant, transform := range variants {
			typed := transform(wizard.phrase)
			if typed == wizard.phrase {
				t.Fatalf("%s/%s produced the exact phrase, so it proves nothing", wizard.name, variant)
			}
			t.Run(wizard.name+"/"+variant, func(t *testing.T) {
				m := sendText(wizard.at(), typed)
				updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				if cmd != nil {
					t.Fatalf("%s dispatched with miscased confirmation %q", wizard.name, typed)
				}
				assertContains(t, updated.(Model).View(), "Confirmation mismatch")
			})
		}
	}
}

func TestConfirmationStillRejectsWrongPhraseForEveryWizard(t *testing.T) {
	for _, wizard := range confirmationWizards() {
		t.Run(wizard.name, func(t *testing.T) {
			m := sendText(wizard.at(), wizard.wrong)
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("%s dispatched with wrong phrase %q", wizard.name, wizard.wrong)
			}
			assertContains(t, updated.(Model).View(), "Confirmation mismatch")
		})
	}
}

func TestConfirmationTabKeyProducesValidatingValueForEveryWizard(t *testing.T) {
	for _, wizard := range confirmationWizards() {
		t.Run(wizard.name, func(t *testing.T) {
			m := wizard.at()
			// Type the phrase using tab wherever a space belongs.
			for i, word := range strings.Split(wizard.phrase, " ") {
				if i > 0 {
					m = sendKey(m, tea.KeyTab)
				}
				m = sendText(m, word)
			}
			if !submitsConfirmation(m) {
				t.Fatalf("%s rejected a confirmation typed with tab separators", wizard.name)
			}
		})
	}
}

func TestConfirmationFiltersControlRunesForEveryWizard(t *testing.T) {
	for _, wizard := range confirmationWizards() {
		t.Run(wizard.name, func(t *testing.T) {
			m := sendText(wizard.at(), wizard.phrase)
			m = sendRune(m, '\x07')
			view := m.View()
			if strings.Contains(view, "\x07") {
				t.Fatalf("%s stored a control rune in the confirmation field", wizard.name)
			}
			assertContains(t, view, "["+wizard.phrase+"]")
			if !submitsConfirmation(m) {
				t.Fatalf("%s rejected a correct confirmation after a control rune", wizard.name)
			}
		})
	}
}

func TestConfirmationPanelShowsDelimitersAndLiveMatchIndicator(t *testing.T) {
	for _, wizard := range confirmationWizards() {
		t.Run(wizard.name, func(t *testing.T) {
			partial := sendText(wizard.at(), wizard.phrase[:len(wizard.phrase)-3])
			assertContains(t, partial.View(), "not yet matching")
			assertNotContains(t, partial.View(), "matches")

			// A trailing space must remain visible through the delimiters even
			// though normalization accepts it.
			padded := sendText(wizard.at(), wizard.phrase)
			padded = sendKey(padded, tea.KeySpace)
			view := padded.View()
			assertContains(t, view, "["+wizard.phrase+" ]", "matches")
			assertNotContains(t, view, "not yet matching")
		})
	}
}

func TestNormalizeConfirmationCollapsesWhitespaceAndPreservesCase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  PURGE  project   alpha ", "PURGE project alpha"},
		{"PURGE\tproject\talpha", "PURGE project alpha"},
		{"PURGE project alpha", "PURGE project alpha"},
		{"purge project alpha", "purge project alpha"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := normalizeConfirmation(c.in); got != c.want {
			t.Fatalf("normalizeConfirmation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfirmationDispatchSendsCanonicalPhrase(t *testing.T) {
	executor := &fakeProjectArchiveExecutor{}
	m := NewModelWithSnapshotAndProjectArchiveExecutor(projectArchiveSnapshot(), executor)
	m = sendKey(m, tea.KeyEnter)
	m = sendRune(m, 'a')
	m = sendText(m, "backup-archive")
	m = sendKey(m, tea.KeyEnter)
	m = sendText(m, "  ARCHIVE  project  alpha  ")
	m = submitProjectArchiveAndApplyResult(t, m)

	if len(executor.requests) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(executor.requests))
	}
	if executor.requests[0].Confirmation != "ARCHIVE project alpha" {
		t.Fatalf("confirmation = %q, want the canonical phrase", executor.requests[0].Confirmation)
	}
}
