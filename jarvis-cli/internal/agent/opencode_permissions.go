package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Keep the wildcard first: OpenCode permission rules are last-match-wins.
// These command patterns are guardrails, not a shell parser or a sandbox.
const openCodeBashPermission = `{"*":"allow","rm *":"ask","rmdir *":"ask","unlink *":"ask","sudo rm *":"ask","sudo rmdir *":"ask","sudo unlink *":"ask","find * -delete*":"ask","find * -exec rm *":"ask","git push --force*":"ask","git push * --force*":"ask","git push * --force-with-lease*":"ask","git push -f*":"ask","git push * -f*":"ask","git push --delete*":"ask","git push * --delete*":"ask","git push * :*":"ask","git push * +*":"ask","git push --mirror*":"ask","git push * --mirror*":"ask","git commit --amend*":"ask","git commit * --amend*":"ask","git reset --hard*":"ask","git reset * --hard*":"ask","git clean *":"ask","git clean -fdx*":"ask","rm -rf /*":"ask","git rebase*":"ask","git filter-branch*":"ask","git filter-repo*":"ask","git branch -d*":"ask","git branch -D*":"ask","git branch * -d*":"ask","git branch * -D*":"ask","git branch --delete*":"ask","git branch * --delete*":"ask","git checkout -- *":"ask","git restore *":"ask","dd *":"ask","sudo dd *":"ask","mkfs*":"ask","sudo mkfs*":"ask","wipefs *":"ask","sudo wipefs *":"ask","shred *":"ask","sudo shred *":"ask","truncate *":"ask"}`

// openCodeSecretPathPermission is the read and edit policy for known secret
// locations. It names locations, never substrings such as *token*: issue #800
// showed substring wildcards block ordinary source files like Tokenizer.php.
// Rules are last-match-wins, so the .env.example/.env.sample allows follow the
// .env denies and precede the directory denies (secrets/.env.example stays
// denied). Claude Code cannot express these allows because its deny beats
// allow. Singular directories (secret/, token/, credential/) are not listed:
// they denied namespace folders such as src/Security/Token/. Home-level
// locations need no extra form because "*" spans "/" here.
// This is a convenience guardrail, not a security boundary: shell
// access and deletion are not covered. Keep in parity with
// claudeSecretPathPatterns.
const openCodeSecretPathPermission = `{"*":"allow",` +
	`".env":"deny",".env.*":"deny","*.env":"deny","**/*.env":"deny","*.env.*":"deny","**/*.env.*":"deny","**/.env*":"deny",` +
	`".env.example":"allow","*/.env.example":"allow","**/.env.example":"allow",".env.sample":"allow","*/.env.sample":"allow","**/.env.sample":"allow",` +
	`"secrets":"deny","*/secrets":"deny","**/secrets":"deny","secrets/**":"deny","**/secrets/**":"deny",` +
	`"tokens":"deny","*/tokens":"deny","**/tokens":"deny","tokens/**":"deny","**/tokens/**":"deny",` +
	`"credentials":"deny","*/credentials":"deny","**/credentials":"deny","credentials/**":"deny","**/credentials/**":"deny",` +
	`".credentials":"deny","*/.credentials":"deny","**/.credentials":"deny",".credentials/**":"deny","**/.credentials/**":"deny",` +
	`".ssh":"deny","*/.ssh":"deny","**/.ssh":"deny",".ssh/*":"deny","*/.ssh/*":"deny","**/.ssh/**":"deny",` +
	`"id_rsa":"deny","*/id_rsa":"deny","**/id_rsa*":"deny","id_ed25519":"deny","*/id_ed25519":"deny","**/id_ed25519*":"deny",` +
	`"*.pem":"deny","*/*.pem":"deny","**/*.pem":"deny","*.key":"deny","*/*.key":"deny","**/*.key":"deny",` +
	`".npmrc":"deny","*/.npmrc":"deny","**/.npmrc":"deny",".netrc":"deny","*/.netrc":"deny","**/.netrc":"deny",` +
	`".pypirc":"deny","*/.pypirc":"deny","**/.pypirc":"deny",` +
	`"credentials.json":"deny","*/credentials.json":"deny","**/credentials.json":"deny",` +
	`".aws/credentials":"deny","*/.aws/credentials":"deny","**/.aws/credentials":"deny",` +
	`".config/gh/hosts.yml":"deny","*/.config/gh/hosts.yml":"deny","**/.config/gh/hosts.yml":"deny",` +
	`"*.p12":"deny","*/*.p12":"deny","**/*.p12":"deny","*.pfx":"deny","*/*.pfx":"deny","**/*.pfx":"deny",` +
	`"*.token":"deny","*/*.token":"deny","**/*.token":"deny",` +
	`"Library/Keychains":"deny","**/Library/Keychains":"deny","Library/Keychains/**":"deny","**/Library/Keychains/**":"deny"}`

// openCodeRetiredSecretReadDenies are the pre-#800 read rules that denied
// ordinary source files: substring wildcards such as *token* (Tokenizer.php)
// and singular directories such as token/ (src/Security/Token/).
var openCodeRetiredSecretReadDenies = []string{
	"*secret*", "**/*secret*", "*token*", "**/*token*", "*credential*", "**/*credential*",
	"secret", "*/secret", "**/secret", "secret/**", "**/secret/**",
	"token", "*/token", "**/token", "token/**", "**/token/**",
	"credential", "*/credential", "**/credential", "credential/**", "**/credential/**",
}

func openCodeDefaultAgentPermission(edit string) string {
	editPermission := `"` + edit + `"`
	if edit == "allow" {
		// Agent rules follow global rules in last-match order, so a bare
		// "allow" would override the global secret edit denies.
		editPermission = openCodeSecretPathPermission
	}
	return `{"task":"deny","edit":` + editPermission + `,"bash":` + openCodeBashPermission + `}`
}

// migrateOpenCodeSecretReadPolicy upgrades the global read policy written by
// older Jarvis releases. Because existing read policies are preserved whole,
// deep-merge would never remove the retired rules nor add new secret rules.
// Only a Jarvis-shaped policy is touched: allow by default and still
// carrying a retired rule as "deny". Those exact entries are removed and
// missing current rules are inserted after their template predecessor, so the
// last-match order matches a fresh install. Any other policy is user-owned.
func migrateOpenCodeSecretReadPolicy(existing []byte) ([]byte, error) {
	if len(strings.TrimSpace(string(existing))) == 0 {
		return existing, nil
	}
	config, err := parseOrderedJSON(existing)
	if err != nil {
		return nil, err
	}
	if config.object == nil {
		return existing, nil
	}
	permission, ok := config.object.get("permission")
	if !ok || permission.object == nil {
		return existing, nil
	}
	read, ok := permission.object.get("read")
	if !ok || read.object == nil {
		return existing, nil
	}
	if fallback, ok := read.object.get("*"); !ok || fallback.scalar != "allow" {
		return existing, nil
	}
	removed := false
	for _, key := range openCodeRetiredSecretReadDenies {
		if value, ok := read.object.get(key); ok && value.scalar == "deny" {
			read.object.delete(key)
			removed = true
		}
	}
	if !removed {
		return existing, nil
	}
	current, err := parseOrderedJSON([]byte(openCodeSecretPathPermission))
	if err != nil {
		return nil, err
	}
	anchor := read.object.index("*")
	for _, pair := range current.object.pairs {
		if idx := read.object.index(pair.key); idx >= 0 {
			anchor = idx
			continue
		}
		pairs := make([]orderedPair, 0, len(read.object.pairs)+1)
		pairs = append(pairs, read.object.pairs[:anchor+1]...)
		pairs = append(pairs, pair)
		pairs = append(pairs, read.object.pairs[anchor+1:]...)
		read.object.pairs = pairs
		anchor++
	}
	return json.Marshal(config)
}

// Filter only OpenCode generated permission defaults before the normal merge.
// Without ownership metadata, existing policies (including old generated bash
// asks) are user-owned. Do not append rules that change last-match precedence.
func preserveOpenCodePermissionPolicies(existing, patch []byte) ([]byte, error) {
	if len(existing) == 0 {
		return patch, nil
	}
	base, err := parseOrderedJSON(existing)
	if err != nil {
		return nil, fmt.Errorf("parse existing permissions: %w", err)
	}
	generated, err := parseOrderedJSON(patch)
	if err != nil {
		return nil, fmt.Errorf("parse generated permissions: %w", err)
	}
	if base.object == nil || generated.object == nil {
		return nil, fmt.Errorf("OpenCode configuration must be an object")
	}
	preserve := func(dst, src *orderedObject) {
		if dst == nil || src == nil {
			return
		}
		current, exists := dst.get("permission")
		defaults, ok := src.get("permission")
		if !exists || !ok {
			return
		}
		if current.object == nil {
			src.set("permission", current)
			return
		}
		if defaults.object == nil {
			return
		}
		for _, key := range []string{"external_directory", "bash", "read"} {
			if value, exists := current.object.get(key); exists {
				defaults.object.set(key, value)
			}
		}
		// An ordered edit policy is user-owned (or already migrated). A scalar
		// edit merges normally: ask/deny stay strict, a generated bare "allow"
		// upgrades to the secret edit policy.
		if value, exists := current.object.get("edit"); exists && value.object != nil {
			defaults.object.set("edit", value)
		}
	}
	preserve(base.object, generated.object)
	baseAgents, _ := base.object.get("agent")
	generatedAgents, _ := generated.object.get("agent")
	if baseAgents.object != nil && generatedAgents.object != nil {
		for _, pair := range generatedAgents.object.pairs {
			current, exists := baseAgents.object.get(pair.key)
			if exists {
				preserve(current.object, pair.value.object)
			}
		}
	}
	return json.Marshal(generated)
}
