package repository

import "fmt"

// resolvedProjectKeyExpr resolves a stored project spelling to the shared Go
// canonical key. The identity registry is authoritative; spellings it does not
// know yet fall back to the ASCII separator fold below.
//
// SQL must never derive canonical keys with canonical_project_key(). That
// function predates the shared contract and disagrees with it: it keeps dots as
// key characters where projectidentity.Canonical treats them as separators, it
// strips characters the Go contract preserves, and it cannot apply Unicode full
// case folding. Comparing an SQL-derived key against a Go-canonicalized
// argument silently matches nothing.
func resolvedProjectKeyExpr(projectExpr string) string {
	return fmt.Sprintf("COALESCE((SELECT pks.project_key FROM project_identity_spellings pks WHERE pks.spelling = %[1]s), %[2]s)",
		projectExpr, asciiSeparatorFoldExpr(projectExpr))
}

// asciiSeparatorFoldExpr mirrors projectidentity.Canonical for ASCII spellings:
// lower-case, collapse separator runs to '-', drop outer separators. It is the
// identity on keys the Go contract already produced.
//
// It is only ever a fallback for spellings the registry has not recorded, and
// it is deliberately conservative: SQL lower() leaves ß intact where Go folds
// it to ss, so the fold never merges two spellings the Go contract keeps apart.
func asciiSeparatorFoldExpr(projectExpr string) string {
	return fmt.Sprintf("trim(both '-' from regexp_replace(lower(%s), '[[:space:]/_.-]+', '-', 'g'))", projectExpr)
}
