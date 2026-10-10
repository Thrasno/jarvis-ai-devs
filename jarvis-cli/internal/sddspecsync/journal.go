package sddspecsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// JournalName is the spec sync journal file inside a change directory. It is
// written before the first main spec write so an interrupted archive can tell
// an applied sync from an untouched one on rerun.
const JournalName = ".spec-sync-journal.json"

const journalSchema = "jarvis.sdd-spec-sync-journal/v1"

// ErrJournal reports a spec sync journal that is unreadable or whose targets
// match neither their pre-sync nor their post-sync digests.
var ErrJournal = errors.New("spec sync journal does not match the main specs")

// Journal records the digests a spec sync moves every target between.
type Journal struct {
	Schema      string          `json:"schema"`
	Destructive bool            `json:"destructive"`
	Targets     []JournalTarget `json:"targets"`
}

// JournalTarget is one main spec of a journaled sync. BeforeDigest is
// AbsentDigest for a spec the sync creates.
type JournalTarget struct {
	Path         string `json:"path"`
	BeforeDigest string `json:"before_digest"`
	AfterDigest  string `json:"after_digest"`
}

// JournalState is where every journaled target currently is.
type JournalState int

const (
	// JournalNotApplied means every target still holds its before digest.
	JournalNotApplied JournalState = iota + 1
	// JournalApplied means every target holds its after digest.
	JournalApplied
)

// NewJournal records the targets of p.
func NewJournal(p Plan) Journal {
	j := Journal{Schema: journalSchema, Destructive: p.Destructive(), Targets: make([]JournalTarget, 0, len(p.Targets))}
	for _, t := range p.Targets {
		j.Targets = append(j.Targets, JournalTarget{Path: t.MainPath, BeforeDigest: t.BeforeDigest, AfterDigest: t.AfterDigest})
	}
	return j
}

// WriteJournal atomically writes j to name.
func WriteJournal(s Store, name string, j Journal) error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return s.WriteFile(name, append(data, '\n'))
}

// RemoveJournal deletes the journal at name; a missing journal is not an error.
func RemoveJournal(s Store, name string) error {
	if err := s.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ReadJournal reads the journal at name. Every target must be a
// <specsRoot>/<capability>/spec.md path with well-formed digests; anything else
// is an ErrJournal failure.
func ReadJournal(s Store, name, specsRoot string) (Journal, bool, error) {
	data, exists, err := s.ReadFile(name)
	if err != nil {
		return Journal{}, false, &ApplyError{Kind: ErrJournal, Path: name, Cause: err}
	}
	if !exists {
		return Journal{}, false, nil
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return Journal{}, true, &ApplyError{Kind: ErrJournal, Path: name, Cause: err}
	}
	if err := validateJournal(j, specsRoot); err != nil {
		return Journal{}, true, &ApplyError{Kind: ErrJournal, Path: name, Cause: err}
	}
	return j, true, nil
}

func validateJournal(j Journal, specsRoot string) error {
	if j.Schema != journalSchema {
		return fmt.Errorf("unsupported schema %q", j.Schema)
	}
	if len(j.Targets) == 0 {
		return errors.New("journal has no targets")
	}
	seen := map[string]bool{}
	for _, t := range j.Targets {
		rel, ok := strings.CutPrefix(t.Path, specsRoot+"/")
		capability, file, nested := strings.Cut(rel, "/")
		if !ok || !fs.ValidPath(t.Path) || !nested || capability == "" || file != "spec.md" || seen[t.Path] {
			return fmt.Errorf("invalid target path %q", t.Path)
		}
		seen[t.Path] = true
		if (t.BeforeDigest != AbsentDigest && !digestPattern.MatchString(t.BeforeDigest)) || !digestPattern.MatchString(t.AfterDigest) {
			return fmt.Errorf("invalid digests for %q", t.Path)
		}
	}
	return nil
}

// Inspect reports whether every target is at its before or its after digest.
// Any other mix fails with ErrJournal and lists, as recovery targets, every
// target not at its before digest: restoring them makes a rerun sync again.
func (j Journal) Inspect(s Store) (JournalState, error) {
	allBefore, allAfter := true, true
	var recovery []RecoveryTarget
	for _, t := range j.Targets {
		data, exists, err := s.ReadFile(t.Path)
		if err != nil {
			return 0, &ApplyError{Kind: ErrJournal, Path: t.Path, Cause: err}
		}
		current := stateDigest(data, exists)
		allAfter = allAfter && current == t.AfterDigest
		if current != t.BeforeDigest {
			allBefore = false
			recovery = append(recovery, RecoveryTarget{Path: t.Path, ExpectedDigest: t.BeforeDigest})
		}
	}
	switch {
	case allAfter:
		return JournalApplied, nil
	case allBefore:
		return JournalNotApplied, nil
	}
	return 0, &ApplyError{Kind: ErrJournal, Path: recovery[0].Path, Recovery: recovery}
}
