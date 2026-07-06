package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func blockedProjectPredicate(projectExpr string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND pb.canonical_project_key = canonical_project_key(%s))", projectExpr)
}

func unblockedProjectPredicate(projectExpr string) string {
	return "NOT " + blockedProjectPredicate(projectExpr)
}

func checkProjectBlocked(ctx context.Context, db pgxQuerier, project string) error {
	var blocked bool
	if err := db.QueryRow(ctx, "SELECT "+blockedProjectPredicate("$1"), project).Scan(&blocked); err != nil {
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
