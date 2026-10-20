package skills

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// openSpecConfigTemplate returns the openspec/config.yaml template that agents
// copy from the shared OpenSpec convention during sdd-init.
func openSpecConfigTemplate(t *testing.T) string {
	t.Helper()

	content := strings.ReplaceAll(readEmbeddedSkillAsset(t, sharedOpenSpecConventionPath), "\r\n", "\n")
	for _, block := range strings.Split(content, "```yaml\n")[1:] {
		body, _, found := strings.Cut(block, "\n```")
		if found && strings.HasPrefix(body, "schema: spec-driven") {
			return body
		}
	}
	t.Fatalf("expected %s to contain the openspec/config.yaml template block", sharedOpenSpecConventionPath)
	return ""
}

func TestOpenSpecConfigTemplateIsValidYAML(t *testing.T) {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(openSpecConfigTemplate(t)), &config); err != nil {
		t.Fatalf("openspec/config.yaml template is not valid YAML: %v", err)
	}

	for _, key := range []string{"strict_tdd_suggestion", "detection_reason", "strict_tdd"} {
		if _, ok := config[key]; !ok {
			t.Errorf("template is missing top-level %q", key)
		}
	}

	testingSettings, ok := config["testing"].(map[string]any)
	if !ok {
		t.Fatalf("template testing = %#v, want a mapping", config["testing"])
	}
	for _, key := range []string{"test_command", "build_command", "coverage_threshold"} {
		if _, ok := testingSettings[key]; !ok {
			t.Errorf("template testing is missing %q", key)
		}
	}

	rules, ok := config["rules"].(map[string]any)
	if !ok {
		t.Fatalf("template rules = %#v, want a mapping", config["rules"])
	}
	for _, phase := range []string{"proposal", "specs", "design", "tasks", "apply", "verify", "archive"} {
		items, ok := rules[phase].([]any)
		if !ok || len(items) == 0 {
			t.Errorf("template rules.%s = %#v, want a non-empty list of text rules", phase, rules[phase])
			continue
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				t.Errorf("template rules.%s item = %#v, want a text rule", phase, item)
			}
		}
	}
}

func TestStrictTDDReadsTestCommandFromTestingBlock(t *testing.T) {
	strictTDD := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/strict-tdd.md")

	if !strings.Contains(strictTDD, "`openspec/config.yaml` `testing.test_command`") {
		t.Errorf("strict-tdd.md must read the test command from openspec/config.yaml testing.test_command")
	}
	if strings.Contains(strictTDD, "rules.apply.test_command") {
		t.Errorf("strict-tdd.md must not read rules.apply.test_command; rules.<phase> holds text rules only")
	}
}
