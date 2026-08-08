package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/projectkey"
	"github.com/jackc/pgx/v5"
)

// blockedProjectPredicate reports whether a stored project spelling belongs to
// a quarantined project.
//
// Quarantine must fail closed: a spelling the identity registry does not know
// must never read as unblocked. The predicate therefore over-approximates the
// candidate keys for a row instead of trusting a single lookup:
//
//   - the literal spelling recorded on the block itself;
//   - the registry mapping, authoritative once the spelling was registered;
//   - the stored value itself, which already is the shared Go canonical key for
//     every row written through the sync path;
//   - an ASCII separator fold, which reproduces the Go contract for ASCII
//     spellings and covers raw legacy rows that predate the registry.
//
// Over-approximating can only ever block more, never less, so it is safe here.
// Identity resolution (scoping, grouping, joining) must never use it, because
// there over-approximating would mix distinct projects — see
// resolvedProjectKeyExpr.
//
// Known residual: the Go contract applies Unicode full case folding (ß -> ss),
// which SQL cannot reproduce. A non-ASCII legacy spelling is therefore covered
// by the registry (populated on every write and by the startup backfill), not
// by the fold.
func blockedProjectPredicate(projectExpr string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND (pb.project = %[1]s OR pb.canonical_project_key IN (%[1]s, %[2]s, COALESCE((SELECT pbs.project_key FROM project_identity_spellings pbs WHERE pbs.spelling = %[1]s), ''))))", projectExpr, asciiSeparatorFoldExpr(projectExpr))
}

func unblockedProjectPredicate(projectExpr string) string {
	return "NOT " + blockedProjectPredicate(projectExpr)
}

func checkProjectBlocked(ctx context.Context, db pgxQuerier, project string) error {
	var blocked bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM project_blocks WHERE blocked = true AND canonical_project_key = $1)", projectkey.Canonicalize(project)).Scan(&blocked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return wrapPgError(err, "check project block")
	}
	if blocked {
		return ErrProjectBlocked
	}
	return nil
}
