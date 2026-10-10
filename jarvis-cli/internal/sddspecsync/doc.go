// Package sddspecsync merges OpenSpec delta specs into main specs.
//
// It encodes the sdd-archive spec sync rules: ADDED, MODIFIED, REMOVED, and
// RENAMED requirement sections are validated and merged into
// openspec/specs/<capability>/spec.md in a read-only plan, then written with
// before-digest checks and rollback.
package sddspecsync
