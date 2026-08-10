package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// blockedProjectPredicate reports whether a stored project spelling belongs to
// a quarantined project.
//
// The API asks exactly one question about a project: is this literal
// quarantined? It never derives, folds or reconciles a key to answer it, so the
// predicate is plain equality between the stored spelling and the literal an
// admin blocked.
//
// It used to over-approximate across the block's own spelling, the identity
// registry, and an ASCII separator fold, guarded by a COALESCE fallback to the
// empty string. Over-blocking looked safe and was not: the fold quarantined
// unrelated projects that merely spelled their name similarly, and the sentinel
// matched every row whose canonical key was the empty string.
//
// Admins block the exact spelling. Two spellings are one project only once an
// admin has merged them.
func blockedProjectPredicate(projectExpr string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND pb.canonical_project_key = %s)", projectExpr)
}

func unblockedProjectPredicate(projectExpr string) string {
	return "NOT " + blockedProjectPredicate(projectExpr)
}

// checkProjectBlocked is the write-side counterpart of blockedProjectPredicate
// and must resolve the same block for the same literal. It therefore does NOT
// canonicalize its argument: the read predicate compares against a stored
// column and cannot canonicalize its side without deriving identity in SQL, so
// agreement is only possible with both sides literal.
func checkProjectBlocked(ctx context.Context, db pgxQuerier, project string) error {
	var blocked bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM project_blocks WHERE blocked = true AND canonical_project_key = $1)", project).Scan(&blocked); err != nil {
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
