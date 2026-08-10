package repository

import (
	"context"
	"sort"
	"strings"
)

// ProjectKeyLockRepository serializes concurrent work on the same project.
type ProjectKeyLockRepository interface {
	// LockProjectKeys takes an advisory lock per project literal. Callers pass
	// project spellings; the lock is scoped to the exact literal, because that
	// is what a project is here.
	LockProjectKeys(ctx context.Context, projects []string) error
}

// ProjectLockKeys returns the distinct project literals to lock, sorted so any
// two transactions acquire overlapping locks in the same order and cannot
// deadlock against each other.
//
// It deliberately does NOT canonicalize. Locking on a derived key made two
// distinct projects contend on one lock, and it kept a canonicalizer alive in
// the module for a call site that had no business deriving identity at all.
//
// service.distinctProjects dedupes the same literals for the quarantine
// precheck and keeps them in FIRST-SEEN order instead. The orderings differ on
// purpose: this one is sorted because sort order is what makes two overlapping
// transactions take the same locks in the same sequence and not deadlock, and
// that one is first-seen because it feeds diagnostics naming the offending
// project, which should read in the order the request presented them. Neither
// ordering is safe in the other's place.
func ProjectLockKeys(projects []string) []string {
	seen := make(map[string]struct{}, len(projects))
	keys := make([]string, 0, len(projects))
	for _, project := range projects {
		if strings.TrimSpace(project) == "" {
			continue
		}
		if _, ok := seen[project]; ok {
			continue
		}
		seen[project] = struct{}{}
		keys = append(keys, project)
	}
	sort.Strings(keys)
	return keys
}
