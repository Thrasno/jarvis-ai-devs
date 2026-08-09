package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// callArgument matches the text between a call's opening parenthesis and a
// token inside it, at any nesting depth.
//
// It consumes anything except a CLOSING parenthesis. Opening ones are allowed,
// so the scan descends into nested calls; the closing one is the wall, so the
// scan can never leave the call it started in and read a token belonging to the
// next one. That is what makes a match mean "this call mentions a project".
//
// The earlier version allowed at most one fully-closed nested call, which let
// it step OVER a nested call but never into one — so lower(coalesce(project))
// and regexp_replace(btrim(project)) both read as clean, and the fold this
// guard exists to prevent was spelled with exactly that shape.
const callArgument = `(?:[^()]|\()*?`

// screamingIdentifier matches when the token a rule landed on is part of a
// SCREAMING_SNAKE_CASE name — an environment variable or a constant, never a
// column. Applied to the match, so it can only ever narrow a rule.
//
// Descending into nested calls made this necessary: reading a config flag named
// JARVIS_ENABLE_PROJECT_BLOCK_ADMIN through strings.EqualFold is not a project
// predicate, and a guard that reports it is a guard someone deletes.
var screamingIdentifier = regexp.MustCompile(`[A-Z]_[A-Z_]*$`)

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
		pattern: regexp.MustCompile(`(?i)(drop\s+table\s+(if\s+exists\s+)?|to_regclass\s*\(\s*')?project_identity_spellings`),
		allowed: regexp.MustCompile(`(?i)^(drop|to_regclass)`),
		why: "the spelling registry was dropped in migration 021; joining it let one project read another's rows. " +
			"Only removing it (DROP TABLE) or asserting its absence (to_regclass) may name it",
	},
	{
		pattern: regexp.MustCompile(`(?i)(create\s+table\s+(if\s+not\s+exists\s+)?|` +
			`create\s+index\s+(if\s+not\s+exists\s+)?idx_|\bon\s+|` +
			`drop\s+table\s+(if\s+exists\s+)?|to_regclass\s*\(\s*')?project_identities`),
		allowed: regexp.MustCompile(`(?i)^(create|on|drop|to_regclass)`),
		why: "the identity registry was dropped in migration 022; its one reader answered 404 for any literal " +
			"absent from it, which is the API deciding which projects are real. Migration 019 still creates it " +
			"(CREATE TABLE, CREATE INDEX) as the record of a schema that shipped, and 022 removes it in the same " +
			"boot pass. Only creating it there, removing it (DROP TABLE) or asserting its absence (to_regclass) " +
			"may name it — never a FROM, JOIN, INSERT or UPDATE",
	},
	{
		pattern: regexp.MustCompile(`(?i)(drop\s+function\s+(if\s+exists\s+)?|to_regprocedure\s*\(\s*')?canonical_project_key\s*\(`),
		allowed: regexp.MustCompile(`(?i)^(drop|to_regprocedure)`),
		why: "the SQL key function was dropped in migration 021; it diverged from the Go contract it shadowed. " +
			"Removing it (DROP FUNCTION) or asserting its absence (to_regprocedure) is fine. " +
			"The project_blocks column of the same name is a stored literal and is fine",
	},
	{
		pattern: regexp.MustCompile(`(?i)create\s+(or\s+replace\s+)?function\s+[\w.]*(project|canonical|slug|fold|normali[sz]e)[\w.]*\s*\([^)]*\)\s*returns\s+(text|varchar|character\s+varying|citext|name|char)`),
		why: "a SQL function returning a string type is how the key fold was spelled, and renaming it changed nothing. " +
			"Project identity is resolved in Go, by the daemon, and never in the database",
	},
	{
		pattern: regexp.MustCompile(`(?i)\bnormalize\s*\(` + callArgument + `project`),
		allowed: screamingIdentifier,
		why:     "normalize() folds Unicode spellings of a project into one; the stored literal is compared byte for byte",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b[\w.]*project\w*\s+collate\b|\bcollate\b[^,;)]*\bproject`),
		why: "a non-binary collation makes = itself a fold, so the predicate stops being plain equality " +
			"without any function appearing in it",
	},
	{
		pattern: regexp.MustCompile(`(?i)alter\s+column\s+[\w.]*project\w*\s+(set\s+data\s+)?type\s+citext`),
		why: "citext makes every comparison on the column case-insensitive, which folds identity in the schema " +
			"rather than in any query",
	},
	{
		pattern: regexp.MustCompile(`(?i)\b(` + foldFunctions + `)\s*\(` + callArgument + `project`),
		allowed: screamingIdentifier,
		why: "a project predicate must be plain equality on the stored literal, not a fold. " +
			"This covers the canonical_project_key column as well as the project column",
	},
	{
		pattern: regexp.MustCompile(`(?i)\bbtrim\s*\(` + callArgument + `project` + callArgument + `,`),
		allowed: screamingIdentifier,
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

// TestNoProjectIdentityDerivationInAPISources is a cheap hint, not a guarantee.
//
// The guarantee is project_scope_behaviour_test.go, which stores rows under one
// spelling and requires every project-scoped path to answer for that spelling
// and no other. That is a claim about what the code DOES, so it holds however a
// fold is written, wherever it lives, and whether or not its source is in this
// repository. This file only reads the source, so it is worth exactly the list
// of spellings someone thought of, and it has now twice been shipped claiming
// coverage it did not have.
//
// It earns its place by failing fast and by naming the construct, and by seeing
// one thing behaviour cannot: a fold added to code no test exercises yet. It
// runs in a quarter of a second against every .go and .sql file; the behavioural
// suite needs a container. Treat a failure here as real and a pass here as
// nothing at all.
//
// Concretely, this scan does NOT see:
//   - a fold assembled at runtime — fmt.Sprintf over a name in a constant, or
//     split across statements so no single expression shows both halves;
//   - a fold applied by a dependency, or by SQL living outside this repository:
//     a view, a trigger, a rule, or a DBA-created function this module calls;
//   - a call to an already-existing fold function, under any name. The rules
//     below catch a DEFINITION whose name suggests identity, which is a guess;
//   - a fold spelled with a function nobody listed in foldFunctions, or reached
//     through an operator rather than a call;
//   - anything at all in testdata/, which is skipped.
//
// Every one of those is caught behaviourally, by the rows the query returns.
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
		if info.IsDir() {
			// testdata/ is never compiled and never applied to a database. The
			// fixture that reproduces the released fold lives there precisely
			// so it can spell the forbidden construct without being one.
			if info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		isSQL := strings.HasSuffix(path, ".sql")
		if (!isSQL && !strings.HasSuffix(path, ".go")) || path == self {
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
		name:   "the dropped identity registry read as the list of projects that exist",
		sample: "const q = `SELECT 1 FROM project_identities WHERE project_key = $1`",
	},
	{
		name:   "the dropped identity registry joined into a memory query",
		sample: "const q = `SELECT m.id FROM memories m JOIN project_identities pi ON pi.project_key = m.project`",
	},
	{
		name:   "a write back into the dropped identity registry",
		isSQL:  true,
		sample: "INSERT INTO project_identities (project_key, first_spelling, first_seen_at) VALUES ($1, $2, now());",
	},
	{
		name:   "pattern match on a project column",
		sample: "const q = `SELECT 1 FROM memories WHERE project ILIKE $1`",
	},
	{
		name:   "pattern match on the canonical_project_key column",
		sample: "const q = `SELECT 1 FROM project_blocks WHERE canonical_project_key LIKE $1`",
	},
	// Everything below was shown to slip past an earlier version of this guard.
	// They are the reason the file no longer claims to be the guarantee.
	{
		name:   "fold reaching a project through a nested call",
		sample: "const q = `SELECT 1 FROM memories WHERE lower(coalesce(memories.project, '')) = lower($1)`",
	},
	{
		name:   "fold reaching a project through two nested calls",
		isSQL:  true,
		sample: "CREATE UNIQUE INDEX idx_memories_project ON memories (lower(btrim(project)));",
	},
	{
		name:   "fold reaching a project column inside a rewrite",
		isSQL:  true,
		sample: "UPDATE project_blocks SET canonical_project_key = regexp_replace(btrim(project), '[^a-z]+','-','g');",
	},
	{
		name:   "hand-rolled Go fold through nested calls",
		sample: "key := strings.ToLower(strings.TrimSpace(project))",
	},
	{
		name:   "renamed fold returning varchar instead of text",
		isSQL:  true,
		sample: "CREATE FUNCTION project_fold(input text) RETURNS varchar LANGUAGE sql AS $$ SELECT lower(input) $$;",
	},
	{
		name:   "renamed fold returning citext",
		isSQL:  true,
		sample: "CREATE FUNCTION canonical_slug(input text) RETURNS citext LANGUAGE sql AS $$ SELECT input $$;",
	},
	{
		name:   "Unicode normalization of a project",
		isSQL:  true,
		sample: "SELECT 1 FROM memories WHERE normalize(project, NFKC) = $1;",
	},
	{
		name:   "case-insensitive collation on a project predicate",
		isSQL:  true,
		sample: "SELECT 1 FROM memories WHERE project COLLATE \"und-u-ks-level2\" = $1;",
	},
	{
		name:   "folding the project column by changing its type",
		isSQL:  true,
		sample: "ALTER TABLE memories ALTER COLUMN project TYPE citext;",
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
		name:   "migration 022 removing the identity registry",
		isSQL:  true,
		sample: "DROP TABLE IF EXISTS project_identities;",
	},
	{
		name:   "migration 019 creating the registry it later loses",
		isSQL:  true,
		sample: "CREATE TABLE IF NOT EXISTS project_identities (\n    project_key text PRIMARY KEY\n);\n\nCREATE INDEX IF NOT EXISTS idx_project_identities_first_seen\n    ON project_identities (first_seen_at ASC, project_key ASC);",
	},
	{
		name:   "a test asserting the identity registry does not exist",
		sample: "pool.QueryRow(ctx, `SELECT to_regclass('project_identities') IS NOT NULL`)",
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
	{
		name:   "a text-returning function that has nothing to do with projects",
		isSQL:  true,
		sample: "CREATE FUNCTION audit_actor(actor uuid) RETURNS text LANGUAGE sql AS $$ SELECT 'system' $$;",
	},
	{
		name:   "a fold whose nested call ends before the project token",
		sample: "const q = `SELECT lower(u.username) AS username_key FROM users u WHERE u.project_key = $1`",
	},
	{
		name:   "a collation on a column that is not a project",
		isSQL:  true,
		sample: "SELECT 1 FROM users WHERE username COLLATE \"C\" = $1;",
	},
	{
		name:   "reading a config flag whose env var name contains PROJECT",
		sample: "enabled := strings.EqualFold(strings.TrimSpace(os.Getenv(\"JARVIS_ENABLE_PROJECT_BLOCK_ADMIN\")), \"true\")",
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
