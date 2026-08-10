package repository

// relocationSource normalises the from-project precondition that gates every
// project correction on the sync_id conflict branches.
//
// The precondition is "name the literal the row currently holds, or move
// nothing". Two values mean "do not move me": an empty from-project, and one
// equal to the project being written — the latter asks to move a project onto
// itself, which the memory path rejects outright as a malformed instruction
// (reprojectInstructionError). But `WHERE sessions.project = from_project` is
// TRIVIALLY TRUE for a self-move, so the conflict branch fired anyway and took
// its whole SET clause with it, including synced_at = now().
//
// That is a trap for a client nobody has written yet. A daemon populating
// from_project unconditionally — the natural reading of "the project this row
// currently holds" — would push every ordinary re-push back into every
// teammate's ListSessionsSince window, forever. Collapsing a self-move to the
// empty sentinel makes it match nothing, which is what it asked for.
//
// It is a no-op rather than an error because a session or prompt push has no
// per-row result vocabulary the way a mutation does: rejecting would fail the
// entire sync over an instruction that requests no change.
func relocationSource(project, fromProject string) string {
	if fromProject == project {
		return ""
	}
	return fromProject
}
