package agent

import (
	"encoding/json"
	"fmt"
)

// Keep the wildcard first: OpenCode permission rules are last-match-wins.
// These command patterns are guardrails, not a shell parser or a sandbox.
const openCodeBashPermission = `{"*":"allow","rm *":"ask","rmdir *":"ask","unlink *":"ask","sudo rm *":"ask","sudo rmdir *":"ask","sudo unlink *":"ask","find * -delete*":"ask","find * -exec rm *":"ask","git push --force*":"ask","git push * --force*":"ask","git push * --force-with-lease*":"ask","git push -f*":"ask","git push * -f*":"ask","git push --delete*":"ask","git push * --delete*":"ask","git push * :*":"ask","git push * +*":"ask","git push --mirror*":"ask","git push * --mirror*":"ask","git commit --amend*":"ask","git commit * --amend*":"ask","git reset --hard*":"ask","git reset * --hard*":"ask","git clean *":"ask","git clean -fdx*":"ask","rm -rf /*":"ask","git rebase*":"ask","git filter-branch*":"ask","git filter-repo*":"ask","git branch -d*":"ask","git branch -D*":"ask","git branch * -d*":"ask","git branch * -D*":"ask","git branch --delete*":"ask","git branch * --delete*":"ask","git checkout -- *":"ask","git restore *":"ask","dd *":"ask","sudo dd *":"ask","mkfs*":"ask","sudo mkfs*":"ask","wipefs *":"ask","sudo wipefs *":"ask","shred *":"ask","sudo shred *":"ask","truncate *":"ask"}`

func openCodeDefaultAgentPermission(edit string) string {
	return `{"task":"deny","edit":"` + edit + `","bash":` + openCodeBashPermission + `}`
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
