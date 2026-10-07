package agent

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"
)

func permissionConfig(t *testing.T, base string) orderedValue {
	t.Helper()
	a := &OpenCodeAgent{home: t.TempDir(), templatesFS: os.DirFS("../..")}
	if err := os.MkdirAll(a.ConfigDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.settingsPath(), []byte(base), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := a.MergeGeneratedConfig(defaultRuntimePhaseModels()); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(a.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	v, err := parseOrderedJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func permissionAt(t *testing.T, v orderedValue, keys ...string) orderedValue {
	t.Helper()
	for _, key := range keys {
		if v.object == nil {
			t.Fatalf("missing object for %s", key)
		}
		next, ok := v.object.get(key)
		if !ok {
			t.Fatalf("missing %s", key)
		}
		v = next
	}
	return v
}

// Evaluate only the simple wildcard subset used below, in OpenCode's last-match order.
func shellDecision(t *testing.T, policy orderedValue, command string) string {
	t.Helper()
	decision := ""
	for _, rule := range policy.object.pairs {
		// filepath/path Match does not let * span slashes; OpenCode does.
		pattern := rule.key
		commandForMatch := command
		pattern = replaceSlashes(pattern)
		commandForMatch = replaceSlashes(commandForMatch)
		matches, err := path.Match(pattern, commandForMatch)
		if err != nil {
			t.Fatal(err)
		}
		if matches {
			decision, _ = rule.value.scalar.(string)
		}
	}
	return decision
}
func replaceSlashes(s string) string { return strings.ReplaceAll(s, "/", "|") }

func TestOpenCodePermissionDefaults(t *testing.T) {
	v := permissionConfig(t, `{}`)
	if got := permissionAt(t, v, "permission", "external_directory").scalar; got != "allow" {
		t.Fatalf("external_directory = %v", got)
	}
	global := permissionAt(t, v, "permission", "bash")
	for command, want := range map[string]string{
		"go test ./...": "allow", "git commit -m fix": "allow", "git push origin main": "allow", "ls /other/project": "allow",
		"rm file": "ask", "rm -rf /other/project": "ask", "rmdir empty": "ask", "unlink file": "ask",
		"git push -f origin main": "ask", "git push origin --force-with-lease": "ask", "git reset --hard HEAD": "ask",
		"git clean -fd": "ask", "git rebase main": "ask", "git branch -D old": "ask", "git branch --delete old": "ask",
		"git push origin --delete old": "ask", "git push origin :old": "ask", "git filter-branch --all": "ask",
		"git commit --amend": "ask", "git push origin +main": "ask", "git push --mirror origin": "ask",
		"find . -delete": "ask", "dd if=image of=/dev/sda": "ask", "mkfs.ext4 /dev/sda": "ask", "sudo rm -rf dir": "ask",
	} {
		t.Run(command, func(t *testing.T) {
			if got := shellDecision(t, global, command); got != want {
				t.Fatalf("decision = %s, want %s", got, want)
			}
		})
	}
	read := permissionAt(t, v, "permission", "read").toAny().(map[string]any)
	assertOpenCodeReadDenyCoverage(t, read)
	agents := permissionAt(t, v, "agent")
	for _, pair := range agents.object.pairs {
		if pair.key == "sdd-orchestrator" {
			continue
		}
		p := permissionAt(t, pair.value, "permission")
		bash := permissionAt(t, p, "bash")
		g, _ := json.Marshal(global)
		b, _ := json.Marshal(bash)
		if string(g) != string(b) {
			t.Errorf("%s has inconsistent bash policy", pair.key)
		}
		if permissionAt(t, p, "task").scalar != "deny" {
			t.Errorf("%s task elevated", pair.key)
		}
		switch pair.key {
		case "sdd-explore", "sdd-verify", "sdd-onboard", "jd-judge-a", "jd-judge-b", "review-risk", "review-readability", "review-reliability", "review-resilience":
			if permissionAt(t, p, "edit").scalar != "deny" {
				t.Errorf("%s edit elevated", pair.key)
			}
		}
	}
}

func TestOpenCodePermissionPreservation(t *testing.T) {
	for _, policy := range []string{`"allow"`, `"ask"`, `"deny"`, `{"/safe/*":"allow","*":"ask"}`, `{"*":"deny","/safe/*":"allow"}`} {
		t.Run(policy, func(t *testing.T) {
			base := `{"permission":{"external_directory":` + policy + `,"bash":` + policy + `,"read":` + policy + `},"agent":{"sdd-apply":{"permission":{"bash":` + policy + `}},"jd-judge-a":{"permission":{"bash":"ask"}}}}`
			v := permissionConfig(t, base)
			original, _ := parseOrderedJSON([]byte(base))
			for _, keys := range [][]string{{"permission", "external_directory"}, {"permission", "bash"}, {"permission", "read"}, {"agent", "sdd-apply", "permission", "bash"}, {"agent", "jd-judge-a", "permission", "bash"}} {
				got, _ := json.Marshal(permissionAt(t, v, keys...))
				want, _ := json.Marshal(permissionAt(t, original, keys...))
				if string(got) != string(want) {
					t.Errorf("%v = %s, want %s", keys, got, want)
				}
			}
		})
	}
	for _, scalar := range []string{`"allow"`, `"ask"`, `"deny"`} {
		t.Run("global"+scalar, func(t *testing.T) {
			v := permissionConfig(t, `{"permission":`+scalar+`,"agent":{"sdd-apply":{"permission":`+scalar+`}}}`)
			for _, keys := range [][]string{{"permission"}, {"agent", "sdd-apply", "permission"}} {
				got, _ := json.Marshal(permissionAt(t, v, keys...))
				if string(got) != scalar {
					t.Errorf("%v = %s", keys, got)
				}
			}
		})
	}
	// Missing external-directory defaults are filled without changing unrelated user rules.
	v := permissionConfig(t, `{"permission":{"bash":"ask"}}`)
	if permissionAt(t, v, "permission", "external_directory").scalar != "allow" {
		t.Fatal("missing external-directory default")
	}
}
