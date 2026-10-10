package sddspecsync

import (
	"errors"
	"testing"
)

const testJournalPath = testChangeRoot + "/" + JournalName

func TestJournalRoundTripAndInspect(t *testing.T) {
	plan, files := applyFixture(t)
	store := newMemStore(files)
	if err := WriteJournal(store, testJournalPath, NewJournal(plan)); err != nil {
		t.Fatalf("WriteJournal() error = %v", err)
	}
	journal, found, err := ReadJournal(store, testJournalPath, testSpecsRoot)
	if err != nil || !found || len(journal.Targets) != len(plan.Targets) {
		t.Fatalf("ReadJournal() = %+v, %t, %v", journal, found, err)
	}
	if state, err := journal.Inspect(store); err != nil || state != JournalNotApplied {
		t.Fatalf("Inspect() before Apply = %v, %v; want JournalNotApplied", state, err)
	}
	if _, err := Apply(plan, store); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if state, err := journal.Inspect(store); err != nil || state != JournalApplied {
		t.Fatalf("Inspect() after Apply = %v, %v; want JournalApplied", state, err)
	}

	// Revert only the first target: the journal is now neither applied nor
	// pending, and every target not at its before digest needs recovery.
	first := plan.Targets[0]
	if first.Existed {
		store.files[first.MainPath] = first.Before
	} else {
		delete(store.files, first.MainPath)
	}
	_, err = journal.Inspect(store)
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || !errors.Is(err, ErrJournal) || applyErr.Code() != CodeRecoveryRequired {
		t.Fatalf("Inspect() mixed = %v, want ErrJournal with %s", err, CodeRecoveryRequired)
	}
	if len(applyErr.Recovery) != len(plan.Targets)-1 {
		t.Fatalf("Recovery = %+v", applyErr.Recovery)
	}
	for i, target := range applyErr.Recovery {
		if want := plan.Targets[i+1]; target.Path != want.MainPath || target.ExpectedDigest != want.BeforeDigest {
			t.Fatalf("Recovery[%d] = %+v, want %s at %s", i, target, want.MainPath, want.BeforeDigest)
		}
	}

	if err := RemoveJournal(store, testJournalPath); err != nil {
		t.Fatalf("RemoveJournal() error = %v", err)
	}
	if _, found, err := ReadJournal(store, testJournalPath, testSpecsRoot); found || err != nil {
		t.Fatalf("ReadJournal() after removal = %t, %v", found, err)
	}
}

func TestReadJournalRejectsInvalidJournals(t *testing.T) {
	digest := "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	target := func(path, before, after string) string {
		return `{"path":"` + path + `","before_digest":"` + before + `","after_digest":"` + after + `"}`
	}
	journal := func(schema string, targets ...string) string {
		out := `{"schema":"` + schema + `","targets":[`
		for i, t := range targets {
			if i > 0 {
				out += ","
			}
			out += t
		}
		return out + "]}"
	}
	for _, tt := range []struct {
		name string
		data string
	}{
		{name: "not JSON", data: "{"},
		{name: "unknown schema", data: journal("other/v1", target(authPath, AbsentDigest, digest))},
		{name: "no targets", data: journal(journalSchema)},
		{name: "target outside the specs root", data: journal(journalSchema, target("README.md", AbsentDigest, digest))},
		{name: "target escaping the specs root", data: journal(journalSchema, target(testSpecsRoot+"/../x/spec.md", AbsentDigest, digest))},
		{name: "target not a capability spec", data: journal(journalSchema, target(testSpecsRoot+"/auth/notes.md", AbsentDigest, digest))},
		{name: "duplicate target", data: journal(journalSchema, target(authPath, AbsentDigest, digest), target(authPath, AbsentDigest, digest))},
		{name: "malformed before digest", data: journal(journalSchema, target(authPath, "sha256:xyz", digest))},
		{name: "absent after digest", data: journal(journalSchema, target(authPath, digest, AbsentDigest))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore(map[string]string{testJournalPath: tt.data})
			_, found, err := ReadJournal(store, testJournalPath, testSpecsRoot)
			var applyErr *ApplyError
			if !found || !errors.Is(err, ErrJournal) || !errors.As(err, &applyErr) || applyErr.Code() != CodeRecoveryRequired {
				t.Fatalf("ReadJournal() = %t, %v; want ErrJournal with %s", found, err, CodeRecoveryRequired)
			}
		})
	}
}
