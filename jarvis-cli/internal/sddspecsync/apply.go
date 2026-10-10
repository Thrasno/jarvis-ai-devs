package sddspecsync

import (
	"bytes"
	"errors"
	"io/fs"
	"sort"
)

// Store reads and writes main specs by slash-separated path. WriteFile must
// be atomic per file and create missing parent directories.
type Store interface {
	ReadFile(name string) (data []byte, exists bool, err error)
	WriteFile(name string, data []byte) error
	Remove(name string) error
}

// Written records one main spec Apply wrote.
type Written struct {
	Path         string
	BeforeDigest string
	Digest       string
	Created      bool
}

// Result lists the written main specs in plan order.
type Result struct {
	Written []Written
}

var (
	errDigestMismatch   = errors.New("post-write digest mismatch")
	errConcurrentEdit   = errors.New("target changed after this sync wrote it")
	errRollbackMismatch = errors.New("rollback did not restore the captured bytes")
)

// touched is a target Apply may have written, with the bytes it observed
// after writing (the planned bytes until a read-back says otherwise).
type touched struct {
	target  Target
	written []byte
}

// Apply writes every planned target. It first confirms every before-digest
// still matches and writes nothing when one is stale. Each target is
// re-checked immediately before its write and verified after it. On any
// failure the targets already touched are rolled back to their captured
// before-bytes (created targets are removed); a target edited by someone else
// since this sync wrote it is never overwritten and is reported for recovery.
func Apply(p Plan, s Store) (Result, error) {
	if err := validatePlan(p); err != nil {
		return Result{}, err
	}
	for _, t := range p.Targets {
		if err := checkBefore(s, t); err != nil {
			return Result{}, err
		}
	}

	var done []touched
	var result Result
	for _, t := range p.Targets {
		if err := checkBefore(s, t); err != nil {
			return Result{}, rollback(s, done, err)
		}
		done = append(done, touched{target: t, written: t.After})
		if err := s.WriteFile(t.MainPath, t.After); err != nil {
			return Result{}, rollback(s, done, &ApplyError{Kind: ErrWriteFailed, Path: t.MainPath, Cause: err})
		}
		observed, err := checkAfter(s, t)
		if observed != nil {
			done[len(done)-1].written = observed
		}
		if err != nil {
			return Result{}, rollback(s, done, err)
		}
		result.Written = append(result.Written, Written{Path: t.MainPath, BeforeDigest: t.BeforeDigest, Digest: t.AfterDigest, Created: !t.Existed})
	}
	for _, t := range p.Targets {
		if _, err := checkAfter(s, t); err != nil {
			return Result{}, rollback(s, done, err)
		}
	}
	return result, nil
}

func validatePlan(p Plan) error {
	seen := map[string]bool{}
	for _, t := range p.Targets {
		invalid := !fs.ValidPath(t.MainPath) || seen[t.MainPath] ||
			t.Existed != (t.Before != nil) ||
			t.BeforeDigest != stateDigest(t.Before, t.Existed) ||
			t.AfterDigest != digest(t.After)
		if invalid {
			return &ApplyError{Kind: ErrInvalidPlan, Path: t.MainPath}
		}
		seen[t.MainPath] = true
	}
	return nil
}

func matchesBefore(t Target, data []byte, exists bool) bool {
	return exists == t.Existed && (!exists || bytes.Equal(data, t.Before))
}

func checkBefore(s Store, t Target) error {
	data, exists, err := s.ReadFile(t.MainPath)
	if err != nil {
		return &ApplyError{Kind: ErrStale, Path: t.MainPath, Cause: err}
	}
	if !matchesBefore(t, data, exists) {
		return &ApplyError{Kind: ErrStale, Path: t.MainPath}
	}
	return nil
}

// checkAfter verifies the written digest and returns the bytes it observed.
func checkAfter(s Store, t Target) ([]byte, error) {
	data, exists, err := s.ReadFile(t.MainPath)
	if err != nil {
		return nil, &ApplyError{Kind: ErrWriteFailed, Path: t.MainPath, Cause: err}
	}
	if !exists || digest(data) != t.AfterDigest {
		return data, &ApplyError{Kind: ErrWriteFailed, Path: t.MainPath, Cause: errDigestMismatch}
	}
	return data, nil
}

// rollback restores touched targets in reverse order and attaches every
// target it could not verifiably restore to failure.Recovery.
func rollback(s Store, done []touched, err error) error {
	var failure *ApplyError
	if !errors.As(err, &failure) {
		failure = &ApplyError{Kind: ErrWriteFailed, Cause: err}
	}
	for i := len(done) - 1; i >= 0; i-- {
		if restoreErr := restore(s, done[i]); restoreErr != nil {
			failure.Recovery = append(failure.Recovery, RecoveryTarget{Path: done[i].target.MainPath, ExpectedDigest: done[i].target.BeforeDigest})
		}
	}
	sort.Slice(failure.Recovery, func(i, j int) bool { return failure.Recovery[i].Path < failure.Recovery[j].Path })
	return failure
}

func restore(s Store, d touched) error {
	t := d.target
	data, exists, err := s.ReadFile(t.MainPath)
	if err != nil {
		return err
	}
	if matchesBefore(t, data, exists) {
		return nil
	}
	if !exists || !bytes.Equal(data, d.written) {
		return errConcurrentEdit
	}
	if t.Existed {
		err = s.WriteFile(t.MainPath, t.Before)
	} else {
		err = s.Remove(t.MainPath)
	}
	if err != nil {
		return err
	}
	data, exists, err = s.ReadFile(t.MainPath)
	if err != nil {
		return err
	}
	if !matchesBefore(t, data, exists) {
		return errRollbackMismatch
	}
	return nil
}
