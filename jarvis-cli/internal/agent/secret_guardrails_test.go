package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// Issue #800: secret guardrails protect known secret locations only. Ordinary
// source files whose names contain token/secret/credential stay usable.

var ordinarySourceFilesNamedLikeSecrets = []string{
	"Tokenizer.php",
	"src/Tokenizer.php",
	"src/Token.php",
	"tests/TokenizerTest.php",
	"AuthTokenService.ts",
	"src/auth/AuthTokenService.ts",
	"src/hooks/useToken.ts",
	"app/Http/Middleware/CsrfTokenMiddleware.php",
	"app/Http/Controllers/SecretaryController.php",
	"src/components/CredentialForm.tsx",
	"internal/auth/token_store.go",
	"pkg/secret_manager.go",
	// Namespace folders: Claude Code matches case-insensitively, so singular
	// directory rules such as token/ denied these (L4).
	"src/Security/Token/JwtToken.php",
	"src/Secret/Vault.php",
	"src/Credential/Store.php",
	"internal/token/parser.go",
}

var secretLocations = []string{
	".env",
	"config/.env",
	".env.local",
	"id_rsa",
	".ssh/id_rsa",
	"secrets/x",
	"app/secrets/x",
	".npmrc",
	"packages/web/.npmrc",
	".netrc",
	".pypirc",
	"credentials.json",
	"config/credentials.json",
	".aws/credentials",
	".config/gh/hosts.yml",
	"certs/client.p12",
	"certs/client.pfx",
	"deploy/api.token",
	"server.pem",
	"server.key",
	"tokens/api",
	"config/credentials/prod",
	".credentials/x",
}

// homeSecretLocations are home-relative secret paths outside the project root.
var homeSecretLocations = []string{
	".aws/credentials",
	".ssh/id_rsa",
	".ssh/config",
	".npmrc",
	".netrc",
	".pypirc",
	".config/gh/hosts.yml",
	".credentials/x",
	"Library/Keychains/login.keychain-db",
	".config/tokens/todoist.env",
}

// retiredSingularDirectoryPatterns are the singular directory rules retired by
// L4: Claude Code matches case-insensitively, so they denied namespace folders.
var retiredSingularDirectoryPatterns = []string{
	"secret", "**/secret", "secret/**", "**/secret/**",
	"token", "**/token", "token/**", "**/token/**",
	"credential", "**/credential", "credential/**", "**/credential/**",
}

var retiredOpenCodeSingularDirectoryKeys = []string{
	"secret", "*/secret", "**/secret", "secret/**", "**/secret/**",
	"token", "*/token", "**/token", "token/**", "**/token/**",
	"credential", "*/credential", "**/credential", "credential/**", "**/credential/**",
}

// retiredSecretWildcards are the substring patterns retired by issue #800.
var retiredSecretWildcards = []string{"*secret*", "**/*secret*", "*token*", "**/*token*", "*credential*", "**/*credential*"}

// legacyClaudeSecretReadDenies is the pre-#800 Jarvis-emitted Read deny list.
var legacyClaudeSecretReadDenies = []string{
	"Read(.env*)", "Read(**/.env*)", "Read(*.env)", "Read(**/*.env)", "Read(*.env.*)", "Read(**/*.env.*)",
	"Read(secrets)", "Read(**/secrets)", "Read(secrets/**)", "Read(**/secrets/**)",
	"Read(secret)", "Read(**/secret)", "Read(secret/**)", "Read(**/secret/**)",
	"Read(tokens)", "Read(**/tokens)", "Read(tokens/**)", "Read(**/tokens/**)",
	"Read(token)", "Read(**/token)", "Read(token/**)", "Read(**/token/**)",
	"Read(credentials)", "Read(**/credentials)", "Read(credentials/**)", "Read(**/credentials/**)",
	"Read(credential)", "Read(**/credential)", "Read(credential/**)", "Read(**/credential/**)",
	"Read(*secret*)", "Read(**/*secret*)", "Read(*token*)", "Read(**/*token*)", "Read(*credential*)", "Read(**/*credential*)",
	"Read(.ssh)", "Read(**/.ssh)", "Read(.ssh/**)", "Read(**/.ssh/**)",
	"Read(id_rsa*)", "Read(**/id_rsa*)", "Read(id_ed25519*)", "Read(**/id_ed25519*)",
	"Read(*.pem)", "Read(**/*.pem)", "Read(*.key)", "Read(**/*.key)",
}

// legacyOpenCodeReadPermission is the pre-#800 Jarvis-emitted OpenCode read policy.
const legacyOpenCodeReadPermission = `{"*": "allow", ".env": "deny", ".env.*": "deny", "*.env": "deny", "**/*.env": "deny", "*.env.*": "deny", "**/*.env.*": "deny", "**/.env*": "deny", "secrets": "deny", "*/secrets": "deny", "**/secrets": "deny", "secrets/**": "deny", "**/secrets/**": "deny", "secret": "deny", "*/secret": "deny", "**/secret": "deny", "secret/**": "deny", "**/secret/**": "deny", "tokens": "deny", "*/tokens": "deny", "**/tokens": "deny", "tokens/**": "deny", "**/tokens/**": "deny", "token": "deny", "*/token": "deny", "**/token": "deny", "token/**": "deny", "**/token/**": "deny", "credentials": "deny", "*/credentials": "deny", "**/credentials": "deny", "credentials/**": "deny", "**/credentials/**": "deny", "credential": "deny", "*/credential": "deny", "**/credential": "deny", "credential/**": "deny", "**/credential/**": "deny", "*secret*": "deny", "**/*secret*": "deny", "*token*": "deny", "**/*token*": "deny", "*credential*": "deny", "**/*credential*": "deny", ".ssh": "deny", "*/.ssh": "deny", "**/.ssh": "deny", ".ssh/*": "deny", "*/.ssh/*": "deny", "**/.ssh/**": "deny", "id_rsa": "deny", "*/id_rsa": "deny", "**/id_rsa*": "deny", "id_ed25519": "deny", "*/id_ed25519": "deny", "**/id_ed25519*": "deny", "*.pem": "deny", "*/*.pem": "deny", "**/*.pem": "deny", "*.key": "deny", "*/*.key": "deny", "**/*.key": "deny"}`

// claudePathPatternMatches approximates Claude Code's gitignore-style path
// rules for project-relative paths: "**/" spans zero or more directories, "*"
// and "?" stay within one segment, a pattern without "/" matches at any depth
// unless anchored, a matched directory covers its contents, and matching
// ignores case (the #800 report was a denied Tokenizer.php against
// Read(**/*token*)).
func claudePathPatternMatches(t *testing.T, pattern, rel string, anchored bool) bool {
	t.Helper()
	var expr strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			expr.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			expr.WriteString(".*")
			i++
		case pattern[i] == '*':
			expr.WriteString("[^/]*")
		case pattern[i] == '?':
			expr.WriteString("[^/]")
		default:
			expr.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	re, err := regexp.Compile("(?i)^" + expr.String() + "$")
	if err != nil {
		t.Fatalf("compile Claude pattern %q: %v", pattern, err)
	}
	segments := strings.Split(rel, "/")
	for end := 1; end <= len(segments); end++ {
		if re.MatchString(strings.Join(segments[:end], "/")) {
			return true
		}
		if !anchored && !strings.Contains(pattern, "/") && re.MatchString(segments[end-1]) {
			return true
		}
	}
	return false
}

// claudeDeniedBy returns the first deny rule for tool that matches target.
// A target starting with "~/" lies outside the project: Claude Code applies
// only "~/"-rooted rules there, anchored at the home directory. Project
// targets ignore "~/" rules.
func claudeDeniedBy(t *testing.T, deny []any, tool, target string) string {
	t.Helper()
	prefix := tool + "("
	homeTarget := strings.HasPrefix(target, "~/")
	for _, raw := range deny {
		rule, ok := raw.(string)
		if !ok || !strings.HasPrefix(rule, prefix) || !strings.HasSuffix(rule, ")") {
			continue
		}
		pattern := strings.TrimSuffix(strings.TrimPrefix(rule, prefix), ")")
		if strings.HasPrefix(pattern, "~/") != homeTarget {
			continue
		}
		if claudePathPatternMatches(t, strings.TrimPrefix(pattern, "~/"), strings.TrimPrefix(target, "~/"), homeTarget) {
			return rule
		}
	}
	return ""
}

// openCodePathDecision evaluates OpenCode's wildcard rules in last-match-wins
// order: "*" spans any characters (including "/"), "?" matches one character.
// Matching ignores case so the not-denied assertions are the strict ones.
func openCodePathDecision(t *testing.T, policies []orderedValue, target string) string {
	t.Helper()
	decision := "allow"
	for _, policy := range policies {
		pairs := []orderedPair{}
		switch {
		case policy.object != nil:
			pairs = policy.object.pairs
		case policy.scalar != nil:
			pairs = []orderedPair{{key: "*", value: policy}}
		}
		for _, pair := range pairs {
			expr := strings.NewReplacer(`\*`, ".*", `\?`, ".").Replace(regexp.QuoteMeta(pair.key))
			re, err := regexp.Compile("(?is)^" + expr + "$")
			if err != nil {
				t.Fatalf("compile OpenCode pattern %q: %v", pair.key, err)
			}
			if re.MatchString(target) {
				decision, _ = pair.value.scalar.(string)
			}
		}
	}
	return decision
}

// openCodeTargets covers both project-relative (edit) and absolute (read)
// path shapes OpenCode may evaluate.
func openCodeTargets(rel string) []string {
	return []string{rel, "/home/dev/project/" + rel}
}

// openCodeHomeTargets covers absolute and project-relative shapes of a path
// under the home directory, outside the project at /home/dev/project.
func openCodeHomeTargets(homeRel string) []string {
	return []string{"/home/dev/" + homeRel, "../" + homeRel}
}

func mergedClaudeDenies(t *testing.T, seed string) ([]any, *ClaudeAgent) {
	t.Helper()
	a := &ClaudeAgent{home: t.TempDir()}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.settingsPath(), []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	settings := readJSONFile(t, a.settingsPath())
	return settings["permissions"].(map[string]any)["deny"].([]any), a
}

func mergedOpenCodeConfig(t *testing.T, seed string) (orderedValue, *OpenCodeAgent) {
	t.Helper()
	a := &OpenCodeAgent{home: t.TempDir(), templatesFS: os.DirFS("../..")}
	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.settingsPath(), []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	data, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	v, err := parseOrderedJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}

// openCodeEffectivePolicies returns the global policy for permission plus the
// agent-level override, in the order OpenCode concatenates them.
func openCodeEffectivePolicies(t *testing.T, config orderedValue, agent, permission string) []orderedValue {
	t.Helper()
	policies := []orderedValue{permissionAt(t, config, "permission", permission)}
	if agent == "" {
		return policies
	}
	agentPermission := permissionAt(t, config, "agent", agent, "permission")
	if override, ok := agentPermission.object.get(permission); ok {
		policies = append(policies, override)
	}
	return policies
}

func TestSecretGuardrails_Claude_OrdinarySourceFilesAreNotDenied(t *testing.T) {
	deny, _ := mergedClaudeDenies(t, `{}`)
	for _, tool := range []string{"Read", "Edit"} {
		for _, rel := range ordinarySourceFilesNamedLikeSecrets {
			if rule := claudeDeniedBy(t, deny, tool, rel); rule != "" {
				t.Errorf("%s(%s) denied by %s", tool, rel, rule)
			}
		}
	}
}

func TestSecretGuardrails_Claude_SecretLocationsDeniedForReadAndEdit(t *testing.T) {
	deny, _ := mergedClaudeDenies(t, `{}`)
	for _, tool := range []string{"Read", "Edit"} {
		// Claude Code keeps .env.* denied: deny beats allow (documented asymmetry).
		for _, rel := range append(append([]string{}, secretLocations...), ".env.example", ".env.sample") {
			if claudeDeniedBy(t, deny, tool, rel) == "" {
				t.Errorf("%s(%s) is not denied", tool, rel)
			}
		}
	}
	for _, wildcard := range append(append([]string{}, retiredSecretWildcards...), retiredSingularDirectoryPatterns...) {
		for _, tool := range []string{"Read", "Edit"} {
			if rule := tool + "(" + wildcard + ")"; countScalar(deny, rule) != 0 {
				t.Errorf("retired rule %s is still emitted", rule)
			}
		}
	}
}

func TestSecretGuardrails_Claude_HomeSecretLocationsDeniedForReadAndEdit(t *testing.T) {
	deny, _ := mergedClaudeDenies(t, `{}`)
	for _, tool := range []string{"Read", "Edit"} {
		for _, rel := range homeSecretLocations {
			if claudeDeniedBy(t, deny, tool, "~/"+rel) == "" {
				t.Errorf("%s(~/%s) is not denied", tool, rel)
			}
		}
		// Home rules are anchored: ordinary home files and projects stay usable.
		for _, rel := range []string{"projects/app/src/Token.php", ".bashrc", ".config/nvim/init.lua"} {
			if rule := claudeDeniedBy(t, deny, tool, "~/"+rel); rule != "" {
				t.Errorf("%s(~/%s) denied by %s", tool, rel, rule)
			}
		}
	}
}

func TestSecretGuardrails_OpenCode_OrdinarySourceFilesAreNotDenied(t *testing.T) {
	config, _ := mergedOpenCodeConfig(t, `{}`)
	for _, permission := range []string{"read", "edit"} {
		for _, agent := range []string{"", "sdd-apply", "jd-fix-agent"} {
			policies := openCodeEffectivePolicies(t, config, agent, permission)
			for _, rel := range ordinarySourceFilesNamedLikeSecrets {
				for _, target := range openCodeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "allow" {
						t.Errorf("agent %q %s %s = %s, want allow", agent, permission, target, got)
					}
				}
			}
		}
	}
}

func TestSecretGuardrails_OpenCode_SecretLocationsDeniedForReadAndEdit(t *testing.T) {
	config, _ := mergedOpenCodeConfig(t, `{}`)
	for _, permission := range []string{"read", "edit"} {
		for _, agent := range []string{"", "sdd-apply", "jd-fix-agent"} {
			policies := openCodeEffectivePolicies(t, config, agent, permission)
			for _, rel := range append(append([]string{}, secretLocations...), "secrets/.env.example") {
				for _, target := range openCodeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "deny" {
						t.Errorf("agent %q %s %s = %s, want deny", agent, permission, target, got)
					}
				}
			}
			for _, rel := range homeSecretLocations {
				for _, target := range openCodeHomeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "deny" {
						t.Errorf("agent %q %s %s = %s, want deny", agent, permission, target, got)
					}
				}
			}
			// OpenCode explicitly allows env templates; Claude Code cannot (deny beats allow).
			for _, rel := range []string{".env.example", ".env.sample", "apps/web/.env.example"} {
				for _, target := range openCodeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "allow" {
						t.Errorf("agent %q %s %s = %s, want allow", agent, permission, target, got)
					}
				}
			}
		}
	}
	for _, permission := range []string{"read", "edit"} {
		policy := permissionAt(t, config, "permission", permission)
		for _, key := range append(append([]string{}, retiredSecretWildcards...), retiredOpenCodeSingularDirectoryKeys...) {
			if _, ok := policy.object.get(key); ok {
				t.Errorf("retired %s rule %q is still emitted", permission, key)
			}
		}
	}
	for _, agent := range []string{"sdd-explore", "sdd-verify", "jd-judge-a"} {
		if got := permissionAt(t, config, "agent", agent, "permission", "edit").scalar; got != "deny" {
			t.Errorf("read-only agent %s edit = %v, want deny", agent, got)
		}
	}
}

func TestClaudeAgent_MergeGeneratedConfig_MigratesRetiredSecretWildcards(t *testing.T) {
	userBefore := []string{"Read(**/private-notes.md)", "Edit(**/*secret*.bak)"}
	userAfter := []string{"Read(**/*token*.bak)", "Bash(custom-danger:*)", "Read(src/Token/**)"}
	seedDeny := append(append(append([]string{}, userBefore...), legacyClaudeSecretReadDenies...), userAfter...)
	for _, pattern := range retiredSingularDirectoryPatterns {
		seedDeny = append(seedDeny, "Edit("+pattern+")")
	}
	seed := map[string]any{
		"theme":       "dark",
		"permissions": map[string]any{"defaultMode": "acceptEdits", "deny": seedDeny},
	}
	seedBytes, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	deny, a := mergedClaudeDenies(t, string(seedBytes))
	first, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig replay: %v", err)
	}
	second, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("migration is not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}

	for _, wildcard := range retiredSecretWildcards {
		if countScalar(deny, "Read("+wildcard+")") != 0 {
			t.Errorf("retired Read(%s) survived migration: %#v", wildcard, deny)
		}
	}
	for _, pattern := range retiredSingularDirectoryPatterns {
		for _, tool := range []string{"Read", "Edit"} {
			if rule := tool + "(" + pattern + ")"; countScalar(deny, rule) != 0 {
				t.Errorf("retired %s survived migration", rule)
			}
		}
	}
	for _, rule := range append(append([]string{}, userBefore...), userAfter...) {
		if countScalar(deny, rule) != 1 {
			t.Errorf("user-owned %q not preserved exactly once: %#v", rule, deny)
		}
	}
	if deny[0] != userBefore[0] || deny[1] != userBefore[1] {
		t.Errorf("user-owned rules lost their position: %#v", deny)
	}
	for _, rule := range []string{"Read(**/.npmrc)", "Edit(**/.npmrc)", "Edit(.env*)", "Read(**/*.token)", "Edit(**/*.token)", "Read(~/.ssh/**)", "Edit(~/.aws/credentials)"} {
		if countScalar(deny, rule) != 1 {
			t.Errorf("current guardrail %q missing or duplicated: %#v", rule, deny)
		}
	}
	for _, tool := range []string{"Read", "Edit"} {
		for _, rel := range []string{"src/Tokenizer.php", "src/auth/AuthTokenService.ts", "src/Security/Token/JwtToken.php", "src/Secret/Vault.php"} {
			if rule := claudeDeniedBy(t, deny, tool, rel); rule != "" {
				t.Errorf("%s(%s) still denied after migration by %s", tool, rel, rule)
			}
		}
		for _, rel := range []string{"~/.aws/credentials", "~/.ssh/id_rsa", "~/.npmrc"} {
			if claudeDeniedBy(t, deny, tool, rel) == "" {
				t.Errorf("%s(%s) not denied after migration", tool, rel)
			}
		}
	}
	settings := readJSONFile(t, a.settingsPath())
	if settings["theme"] != "dark" || settings["permissions"].(map[string]any)["defaultMode"] != "acceptEdits" {
		t.Fatalf("migration lost user settings: %#v", settings)
	}
}

func TestOpenCodeAgent_MergeGeneratedConfig_MigratesRetiredSecretWildcards(t *testing.T) {
	legacyRead := strings.Replace(legacyOpenCodeReadPermission, `"secrets": "deny",`, `"**/private-notes.md": "deny", "secrets": "deny",`, 1)
	legacyRead = strings.Replace(legacyRead, `}`, `, "docs/**": "allow"}`, 1)
	seed := `{"theme":"dark","mcp":{"custom":{"type":"local","command":["custom"]}},"permission":{"external_directory":"allow","bash":{"*":"allow"},"read":` + legacyRead + `},` +
		`"agent":{"sdd-apply":{"permission":{"task":"deny","edit":"allow","bash":{"*":"allow"}}},"user-agent":{"permission":{"edit":"ask"}}}}`
	config, a := mergedOpenCodeConfig(t, seed)
	first, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig replay: %v", err)
	}
	second, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("migration is not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}

	read := permissionAt(t, config, "permission", "read")
	for _, key := range append(append([]string{}, retiredSecretWildcards...), retiredOpenCodeSingularDirectoryKeys...) {
		if _, ok := read.object.get(key); ok {
			t.Errorf("retired read rule %q survived migration", key)
		}
	}
	if read.object.pairs[0].key != "*" || read.object.pairs[0].value.scalar != "allow" {
		t.Errorf("read wildcard default moved: %#v", read.object.pairs[0])
	}
	for key, want := range map[string]string{"**/private-notes.md": "deny", "docs/**": "allow"} {
		if got, ok := read.object.get(key); !ok || got.scalar != want {
			t.Errorf("user-owned read %q = %#v, want %s", key, got, want)
		}
	}
	if got := permissionAt(t, config, "theme").scalar; got != "dark" {
		t.Errorf("theme = %v", got)
	}
	permissionAt(t, config, "mcp", "custom")
	if got := permissionAt(t, config, "agent", "user-agent", "permission", "edit").scalar; got != "ask" {
		t.Errorf("user-owned agent edit = %v, want ask", got)
	}

	for _, agent := range []string{"", "sdd-apply"} {
		readPolicies := openCodeEffectivePolicies(t, config, agent, "read")
		editPolicies := openCodeEffectivePolicies(t, config, agent, "edit")
		for _, policies := range [][]orderedValue{readPolicies, editPolicies} {
			for _, rel := range []string{"src/Tokenizer.php", "src/auth/AuthTokenService.ts", ".env.example", "src/Security/Token/JwtToken.php", "src/Secret/Vault.php"} {
				for _, target := range openCodeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "allow" {
						t.Errorf("agent %q %s = %s after migration, want allow", agent, target, got)
					}
				}
			}
			for _, rel := range []string{".env", "id_rsa", "secrets/x", ".npmrc", "credentials.json", "deploy/api.token", "secrets/.env.example"} {
				for _, target := range openCodeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "deny" {
						t.Errorf("agent %q %s = %s after migration, want deny", agent, target, got)
					}
				}
			}
			for _, rel := range homeSecretLocations {
				for _, target := range openCodeHomeTargets(rel) {
					if got := openCodePathDecision(t, policies, target); got != "deny" {
						t.Errorf("agent %q %s = %s after migration, want deny", agent, target, got)
					}
				}
			}
		}
	}
}

func TestOpenCodeAgent_MergeGeneratedConfig_PreservesUserOwnedRetiredLookalikes(t *testing.T) {
	// A restrictive (deny-by-default) read policy is user-owned, even when it
	// carries a key that Jarvis once emitted; and a non-deny value is user-owned.
	seed := `{"permission":{"read":{"*":"deny","**/*secret*":"deny"},"edit":{"*":"ask","*token*":"allow"}}}`
	config, _ := mergedOpenCodeConfig(t, seed)
	read, _ := json.Marshal(permissionAt(t, config, "permission", "read"))
	if string(read) != `{"*":"deny","**/*secret*":"deny"}` {
		t.Errorf("user-owned read policy changed: %s", read)
	}
	edit, _ := json.Marshal(permissionAt(t, config, "permission", "edit"))
	if string(edit) != `{"*":"ask","*token*":"allow"}` {
		t.Errorf("user-owned edit policy changed: %s", edit)
	}
}

func TestClaudeAgent_MergeGeneratedConfig_EveryEmittedPermissionIsInResetInventory(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
		t.Fatalf("MergeGeneratedConfig: %v", err)
	}
	permissions := readJSONFile(t, a.settingsPath())["permissions"].(map[string]any)
	literals := stringSet(resetSurfaceByID(sddruntime.PlatformClaude, "claude.settings.permissions").Matchers)
	for _, key := range []string{"allow", "deny"} {
		for _, raw := range permissions[key].([]any) {
			if rule := raw.(string); !literals[rule] {
				t.Errorf("emitted permission %q is missing from the reset inventory", rule)
			}
		}
	}
	for _, wildcard := range retiredSecretWildcards {
		if rule := "Read(" + wildcard + ")"; !literals[rule] {
			t.Errorf("retired permission %q must stay in the reset inventory", rule)
		}
	}
	for _, pattern := range retiredSingularDirectoryPatterns {
		for _, tool := range []string{"Read", "Edit"} {
			if rule := tool + "(" + pattern + ")"; !literals[rule] {
				t.Errorf("retired permission %q must stay in the reset inventory", rule)
			}
		}
	}
}
