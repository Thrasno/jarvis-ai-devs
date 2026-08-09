package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// forbiddenIdentityDerivation is the vocabulary of project-identity derivation.
//
// Each entry is a construct that once shipped in this module and caused a real
// defect. They are listed together because all three defects had the same
// shape: one call site kept an old rule while its siblings changed. The
// compiler cannot see into a SQL string literal, so this test is what stops the
// fourth site from being written.
var forbiddenIdentityDerivation = []struct {
	pattern *regexp.Regexp
	why     string
}{
	{
		regexp.MustCompile(`projectidentity\.Canonical\b`),
		"the shared canonicalizer is the daemon's contract; the API stores the literal it receives. " +
			"Only projectidentity.ContractVersion belongs on this side of the wire",
	},
	{
		regexp.MustCompile(`\bprojectkey\.`),
		"internal/projectkey was deleted; it existed only to canonicalize",
	},
	{
		regexp.MustCompile(`project_identity_spellings`),
		"the spelling registry was dropped in migration 021; joining it let one project read another's rows",
	},
	{
		regexp.MustCompile(`canonical_project_key\s*\(`),
		"the SQL key function was dropped in migration 021; it diverged from the Go contract it shadowed. " +
			"(The project_blocks column of the same name is a stored literal and is fine)",
	},
	{
		regexp.MustCompile(`(?i)(lower|upper|regexp_replace|translate)\s*\([^)]*\bproject\b`),
		"a project predicate must be plain equality on the stored literal, not a fold",
	},
	{
		regexp.MustCompile(`(?i)\bproject\s+(I?LIKE|SIMILAR\s+TO)\b`),
		"a project predicate must be plain equality on the stored literal, not a pattern match",
	},
}

// TestNoProjectIdentityDerivationInAPISources pins the one architectural rule
// this module has about projects: the daemon is the sole authority on project
// identity, hive-api stores the literal it receives, and two spellings are the
// same project only when they are byte-for-byte equal.
//
// Every project-scoped SQL predicate in this module lives in a Go string
// literal, so scanning Go sources covers the SQL side the type system cannot.
func TestNoProjectIdentityDerivationInAPISources(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	self, err := filepath.Abs("project_identity_guard_test.go")
	require.NoError(t, err)

	var violations []string
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || path == self {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(source), "\n") {
			for _, forbidden := range forbiddenIdentityDerivation {
				if forbidden.pattern.MatchString(line) {
					rel, _ := filepath.Rel(root, path)
					violations = append(violations,
						rel+": "+strings.TrimSpace(line)+"\n    -> "+forbidden.why)
				}
			}
		}
		return nil
	}))

	require.Empty(t, violations,
		"hive-api must never derive, fold or canonicalize project identity:\n%s",
		strings.Join(violations, "\n"))
}
