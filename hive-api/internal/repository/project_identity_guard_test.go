package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// callArgument matches the argument list of a single call, without escaping it:
// it consumes anything but parentheses, plus at most one nested call. A pattern
// built on it therefore asks "does THIS call mention a project", not "does the
// word project appear somewhere later in the file".
const callArgument = `(?:[^()]|\([^()]*\))*?`

// foldFunctions are the SQL and Go functions that can turn one project spelling
// into another. btrim/trim are absent on purpose: this module uses them to test
// for emptiness, and their cutset forms have their own rule.
const foldFunctions = `lower|upper|initcap|replace|regexp_replace|translate|unaccent|` +
	`strings\.ToLower|strings\.ToUpper|strings\.ReplaceAll|strings\.Replace|strings\.Map|` +
	`strings\.EqualFold|strings\.Title|strings\.ToValidUTF8`

type forbiddenConstruct struct {
	pattern *regexp.Regexp
	// allowed matches the one legitimate spelling of this construct. It is
	// applied to the match itself, so widening it cannot widen the rule.
	allowed *regexp.Regexp
	why     string
}

// forbiddenIdentityDerivation is the vocabulary of project-identity derivation.
//
// Each entry is a construct that once shipped in this module, or a mutation of
// one that a reviewer showed could slip past an earlier version of this guard.
// They are listed together because every defect had the same shape: one call
// site kept an old rule while its siblings changed. The compiler cannot see into
// a SQL string literal or a .sql file, so this test is what stops the next one.
var forbiddenIdentityDerivation = []forbiddenConstruct{
	{
		pattern: regexp.MustCompile(`projectidentity\.Canonical\b`),
		why: "the shared canonicalizer is the daemon's contract; the API stores the literal it receives. " +
			"Only projectidentity.ContractVersion belongs on this side of the wire",
	},
	{
		pattern: regexp.MustCompile(`[\w.]+\s+"github\.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"`),
		allowed: regexp.MustCompile(`^import\b`),
		why: "import projectidentity under its own name. An alias (or a dot import) hides " +
			"projectidentity.Canonical from every other rule in this guard",
	},
	{
		pattern: regexp.MustCompile(`\bprojectkey\.`),
		why:     "internal/projectkey was deleted; it existed only to canonicalize",
	},
	{
		pattern: regexp.MustCompile(`(?i)(drop\s+table\s+(if\s+exists\s+)?)?project_identity_spellings`),
		allowed: regexp.MustCompile(`(?i)^drop`),
		why: "the spelling registry was dropped in migration 021; joining it let one project read another's rows. " +
			"Only migration 021's DROP TABLE may name it",
	},
	{
		pattern: regexp.MustCompile(`(?i)(drop\s+function\s+(if\s+exists\s+)?|to_regprocedure\s*\(\s*')?canonical_project_key\s*\(`),
		allowed: regexp.MustCompile(`(?i)^(drop|to_regprocedure)`),
		why: "the SQL key function was dropped in migration 021; it diverged from the Go contract it shadowed. " +
			"Removing it (DROP FUNCTION) or asserting its absence (to_regprocedure) is fine. " +
			"The project_blocks column of the same name is a stored literal and is fine",
	},
	{
		pattern: regexp.MustCompile(`(?i)create\s+(or\s+replace\s+)?function\s+[\w.]+\s*\([^)]*\)\s*returns\s+text`),
		why: "a SQL function returning text is how the key fold was spelled; renaming it changes nothing. " +
			"Project identity is resolved in Go, by the daemon, and never in the database",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(` + foldFunctions + `)\s*\(` + callArgument + `project`),
		why: "a project predicate must be plain equality on the stored literal, not a fold. " +
			"This covers the canonical_project_key column as well as the project column",
	},
	{
		pattern: regexp.MustCompile(`(?i)\bbtrim\s*\(` + callArgument + `project` + callArgument + `,`),
		why:     "btrim with a cutset folds a project spelling; only the no-cutset emptiness check is allowed",
	},
	{
		pattern: regexp.MustCompile(`(?i)\btrim\s*\(\s*(both|leading|trailing)\b`),
		why:     "trim(both '-' from ...) is the shape of the deleted key fold",
	},
	{
		pattern: regexp.MustCompile(`(?i)(strings\.(ReplaceAll|Replace|Map)\s*\(\s*strings\.To(Lower|Upper)|` +
			`\b(regexp_)?replace\s*\(\s*lower\s*\()`),
		why: "case-folding a value and then rewriting its separators is a hand-rolled copy of the daemon's " +
			"canonicalizer, whatever the variable is called",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b[\w.]*project\w*\s+(not\s+)?(i?like|similar\s+to)\b`),
		why:     "a project predicate must be plain equality on the stored literal, not a pattern match",
	},
}

// stripComments removes comments and keeps everything else, including string
// literals: the SQL this module runs lives inside Go string literals, so the
// literals are exactly what must be scanned. Quotes are tracked only so that a
// "//" or "--" inside a literal does not read as the start of a comment.
func stripComments(source string, isSQL bool) string {
	var out strings.Builder
	runes := []rune(source)
	lineCommentStart := '/'
	if isSQL {
		lineCommentStart = '-'
	}
	for i := 0; i < len(runes); {
		current := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch {
		case current == lineCommentStart && next == lineCommentStart:
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case current == '/' && next == '*':
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i = min(i+2, len(runes))
		case current == '"' || current == '\'' || current == '`':
			out.WriteRune(current)
			i++
			for i < len(runes) {
				if !isSQL && current != '`' && runes[i] == '\\' && i+1 < len(runes) {
					out.WriteRune(runes[i])
					out.WriteRune(runes[i+1])
					i += 2
					continue
				}
				out.WriteRune(runes[i])
				i++
				if runes[i-1] == current {
					break
				}
			}
		default:
			out.WriteRune(current)
			i++
		}
	}
	return out.String()
}

var whitespaceRun = regexp.MustCompile(`\s+`)

// identityViolations reports the forbidden constructs a source file contains.
//
// It matches against the file with comments removed and every whitespace run
// collapsed to one space, so a construct wrapped across lines — by a formatter,
// by string concatenation, or by hand — is as visible as a single-line one.
func identityViolations(source string, isSQL bool) []string {
	flattened := strings.TrimSpace(whitespaceRun.ReplaceAllString(stripComments(source, isSQL), " "))

	var violations []string
	for _, forbidden := range forbiddenIdentityDerivation {
		for _, match := range forbidden.pattern.FindAllString(flattened, -1) {
			if forbidden.allowed != nil && forbidden.allowed.MatchString(match) {
				continue
			}
			violations = append(violations, match+"\n    -> "+forbidden.why)
		}
	}
	return violations
}

// TestNoProjectIdentityDerivationInAPISources pins the one architectural rule
// this module has about projects: the daemon is the sole authority on project
// identity, hive-api stores the literal it receives, and two spellings are the
// same project only when they are byte-for-byte equal.
//
// It scans .sql as well as .go. The Go type system sees neither: a
// project-scoped predicate lives in a Go string literal or in a migration file,
// and the fold this rule exists to prevent was defined in a migration.
//
// What this guard cannot see, and what no static scan of this module could:
//   - a fold assembled at runtime (fmt.Sprintf over a name held in a constant,
//     or split across statements so no single call shows both halves);
//   - a fold applied by a dependency, or by SQL running outside this repository
//     (a view, a trigger, or a DBA-created function this module merely calls);
//   - a fold whose SQL function is renamed AND defined elsewhere — the rule
//     above catches the definition, not a call to an already-existing one.
//
// Those are the reasons the predicate is also covered by behavioural tests
// against a real database, not by this scan alone.
func TestNoProjectIdentityDerivationInAPISources(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	self, err := filepath.Abs("project_identity_guard_test.go")
	require.NoError(t, err)

	scanned := 0
	var violations []string
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		isSQL := strings.HasSuffix(path, ".sql")
		if info.IsDir() || (!isSQL && !strings.HasSuffix(path, ".go")) || path == self {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		rel, _ := filepath.Rel(root, path)
		for _, violation := range identityViolations(string(source), isSQL) {
			violations = append(violations, rel+": "+violation)
		}
		return nil
	}))

	require.Empty(t, violations,
		"hive-api must never derive, fold or canonicalize project identity:\n%s",
		strings.Join(violations, "\n"))

	sqlFiles, err := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, sqlFiles, "the migration files must exist for the scan to mean anything")
	require.Greater(t, scanned, len(sqlFiles), "the walk must reach both .go and .sql sources")
}

// evasions are the ways a future change could reintroduce project-identity
// derivation. Each one is a sample this guard must reject: a guard whose own
// coverage is unproven is how the fold survived three refactors.
var evasions = []struct {
	name   string
	isSQL  bool
	sample string
}{
	{
		name:   "SQL migration redefining the key fold",
		isSQL:  true,
		sample: "CREATE OR REPLACE FUNCTION canonical_project_key(input text)\nRETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT lower($1) $$;",
	},
	{
		name:   "SQL migration folding the project column in an index",
		isSQL:  true,
		sample: "CREATE UNIQUE INDEX idx_memories_project ON memories (lower(project));",
	},
	{
		name:   "SQL migration defining a renamed fold",
		isSQL:  true,
		sample: "CREATE FUNCTION project_fold(input text) RETURNS text LANGUAGE sql AS $$ SELECT btrim(input, '-') $$;",
	},
	{
		name:   "SQL migration trimming a project cutset",
		isSQL:  true,
		sample: "UPDATE project_blocks SET canonical_project_key = btrim(project, '-');",
	},
	{
		name:   "SQL migration trimming a project with the fold's own shape",
		isSQL:  true,
		sample: "SELECT trim(both '-' from regexp_replace(project, '[^a-z]+', '-', 'g')) FROM memories;",
	},
	{
		name:   "fold on the canonical_project_key column",
		sample: "const q = `SELECT 1 FROM project_blocks pb WHERE lower(pb.canonical_project_key) = lower($1)`",
	},
	{
		name:   "fold wrapped across two lines",
		sample: "const q = `SELECT 1 FROM memories WHERE lower(\n\t\tmemories.project) = $1`",
	},
	{
		name:   "fold split across concatenated string literals",
		sample: "const q = \"SELECT 1 FROM memories WHERE lower(memories.\" +\n\t\"project) = $1\"",
	},
	{
		name:   "canonicalizer reached through an aliased import",
		sample: "import (\n\tpi \"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity\"\n)\n\nvar k = pi.Canonical(project).String()",
	},
	{
		name:   "canonicalizer reached through a dot import",
		sample: "import (\n\t. \"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity\"\n)",
	},
	{
		name:   "hand-rolled Go fold on an innocently named variable",
		sample: "func fold(p string) string { return strings.ReplaceAll(strings.ToLower(p), \".\", \"-\") }",
	},
	{
		name:   "hand-rolled SQL fold on an innocently named column",
		sample: "const q = `SELECT replace(lower(p.name), '.', '-') FROM projects p`",
	},
	{
		name:   "the deleted projectkey package",
		sample: "key := projectkey.Canonicalize(create.Project)",
	},
	{
		name:   "the shared canonicalizer called directly",
		sample: "key := projectidentity.Canonical(project).String()",
	},
	{
		name:   "the dropped spelling registry",
		sample: "const q = `SELECT project_key FROM project_identity_spellings WHERE spelling = $1`",
	},
	{
		name:   "pattern match on a project column",
		sample: "const q = `SELECT 1 FROM memories WHERE project ILIKE $1`",
	},
	{
		name:   "pattern match on the canonical_project_key column",
		sample: "const q = `SELECT 1 FROM project_blocks WHERE canonical_project_key LIKE $1`",
	},
}

func TestIdentityGuardRejectsEveryKnownEvasion(t *testing.T) {
	for _, evasion := range evasions {
		t.Run(evasion.name, func(t *testing.T) {
			require.NotEmpty(t, identityViolations(evasion.sample, evasion.isSQL),
				"the guard accepted a known way to reintroduce identity derivation")
		})
	}
}

// legitimate are constructs that read like derivation and are not. The guard
// must accept every one of them: a guard that cries wolf gets deleted, and then
// it guards nothing.
var legitimate = []struct {
	name   string
	isSQL  bool
	sample string
}{
	{
		name:   "migration 021 removing the fold",
		isSQL:  true,
		sample: "DROP FUNCTION IF EXISTS canonical_project_key(text);",
	},
	{
		name:   "migration 021 removing the spelling registry",
		isSQL:  true,
		sample: "DROP TABLE IF EXISTS project_identity_spellings;",
	},
	{
		name:   "a test asserting the fold does not exist",
		sample: "pool.QueryRow(ctx, `SELECT to_regprocedure('canonical_project_key(text)') IS NOT NULL`)",
	},
	{
		name:   "migration 018 trigger function",
		isSQL:  true,
		sample: "CREATE OR REPLACE FUNCTION record_project_quarantine_command() RETURNS trigger AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql;",
	},
	{
		name:   "the stored-literal column of the same name",
		sample: "const q = `SELECT 1 FROM project_blocks WHERE canonical_project_key = $1`",
	},
	{
		name:   "emptiness check on a project column",
		isSQL:  true,
		sample: "CHECK (btrim(project_key) <> '')",
	},
	{
		name:   "trimming a caller's whitespace",
		sample: "if strings.TrimSpace(project) == \"\" { return errEmpty }",
	},
	{
		name:   "unaliased import for the wire contract",
		sample: "import (\n\t\"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity\"\n)\n\nvar v = projectidentity.ContractVersion",
	},
	{
		name:   "a fold on a column that is not a project",
		sample: "const q = `SELECT lower(u.username) AS username_key FROM users u WHERE u.project_key = $1`",
	},
	{
		name:   "a comment describing the forbidden fold",
		isSQL:  true,
		sample: "-- canonical_project_key(text) folded spellings inside SQL; lower(project) is gone too.",
	},
	{
		name:   "a Go comment describing the forbidden fold",
		sample: "// lower(project) and projectidentity.Canonical are both forbidden here.",
	},
}

func TestIdentityGuardAcceptsLegitimateConstructs(t *testing.T) {
	for _, allowed := range legitimate {
		t.Run(allowed.name, func(t *testing.T) {
			require.Empty(t, identityViolations(allowed.sample, allowed.isSQL),
				"the guard rejected a construct this module legitimately uses")
		})
	}
}
