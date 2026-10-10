package sddspecsync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// AbsentDigest is the before-digest of a main spec that does not exist yet.
const AbsentDigest = "absent"

// Plan is the sealed, read-only result of merging every delta spec of a
// change. Nothing is written while building it.
type Plan struct {
	Targets []Target
}

// Target is one main spec the plan will write, in capability order.
type Target struct {
	Capability string
	DeltaPath  string
	MainPath   string
	// Existed reports whether the main spec existed; Before is nil otherwise.
	Existed      bool
	Before       []byte
	BeforeDigest string
	After        []byte
	AfterDigest  string
	Changes      Changes
}

// Destructive reports whether any target deletes requirements.
func (p Plan) Destructive() bool {
	for _, t := range p.Targets {
		if t.Changes.Destructive() {
			return true
		}
	}
	return false
}

// BuildPlan merges every delta spec under <changeRoot>/specs/<capability>/spec.md
// into <specsRoot>/<capability>/spec.md. Paths are slash-separated and
// relative to fsys. Any invalid capability fails the whole plan.
func BuildPlan(fsys fs.FS, changeRoot, specsRoot string) (Plan, error) {
	if !fs.ValidPath(changeRoot) || !fs.ValidPath(specsRoot) {
		return Plan{}, &MergeError{Kind: ErrUnsafePath, Detail: "change and specs roots must be relative slash paths"}
	}
	deltaRoot := path.Join(changeRoot, "specs")
	if err := rejectSymlinks(fsys, deltaRoot); err != nil {
		return Plan{}, err
	}
	entries, err := fs.ReadDir(fsys, deltaRoot)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(entries) == 0) {
		return Plan{}, &MergeError{Kind: ErrNoDeltaSpecs, Detail: deltaRoot}
	}
	if err != nil {
		return Plan{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var plan Plan
	for _, entry := range entries {
		target, err := planCapability(fsys, deltaRoot, specsRoot, entry)
		if err != nil {
			var mergeErr *MergeError
			if errors.As(err, &mergeErr) {
				mergeErr.Capability = entry.Name()
			}
			return Plan{}, err
		}
		plan.Targets = append(plan.Targets, target)
	}
	return plan, nil
}

func planCapability(fsys fs.FS, deltaRoot, specsRoot string, entry fs.DirEntry) (Target, error) {
	capability := entry.Name()
	if strings.HasPrefix(capability, ".") || !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
		return Target{}, &MergeError{Kind: ErrInvalidDelta, Detail: "delta specs must be <capability>/spec.md directories"}
	}
	deltaPath := path.Join(deltaRoot, capability, "spec.md")
	delta, exists, err := readRegular(fsys, deltaPath)
	if err != nil {
		return Target{}, err
	}
	if !exists {
		return Target{}, &MergeError{Kind: ErrInvalidDelta, Detail: deltaPath + " is missing"}
	}
	mainPath := path.Join(specsRoot, capability, "spec.md")
	before, existed, err := readRegular(fsys, mainPath)
	if err != nil {
		return Target{}, err
	}
	after, changes, err := merge(before, existed, delta)
	if err != nil {
		return Target{}, err
	}
	return Target{
		Capability:   capability,
		DeltaPath:    deltaPath,
		MainPath:     mainPath,
		Existed:      existed,
		Before:       before,
		BeforeDigest: stateDigest(before, existed),
		After:        after,
		AfterDigest:  digest(after),
		Changes:      changes,
	}, nil
}

// readRegular reads a regular file, distinguishing absent (nil, false) from
// existing-but-empty ([]byte{}, true). Symlinks anywhere on the path fail.
func readRegular(fsys fs.FS, name string) ([]byte, bool, error) {
	if err := rejectSymlinks(fsys, name); err != nil {
		return nil, false, err
	}
	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, &MergeError{Kind: ErrUnsafePath, Detail: name + " is not a regular file"}
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, false, err
	}
	if data == nil {
		data = []byte{}
	}
	return data, true, nil
}

// rejectSymlinks fails when any existing component of name is a symlink.
// File systems without Lstat support are trusted as is.
func rejectSymlinks(fsys fs.FS, name string) error {
	lfs, ok := fsys.(fs.ReadLinkFS)
	if !ok {
		return nil
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		prefix := path.Join(parts[:i+1]...)
		info, err := lfs.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return &MergeError{Kind: ErrUnsafePath, Detail: prefix + " is a symlink"}
		}
	}
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stateDigest(data []byte, exists bool) string {
	if !exists {
		return AbsentDigest
	}
	return digest(data)
}
