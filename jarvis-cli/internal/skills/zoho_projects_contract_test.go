package skills

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoProjectsSkillRoot = "embed/skills/zoho-projects"

var zohoProjectsContractAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/deluge-tasks.md",
	"references/identifiers-and-metadata.md",
	"references/authentication-limits-and-plans.md",
	"references/lifecycle-and-uncertainty.md",
	"references/sources.md",
}

func readZohoProjectsContractAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoProjectsSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded Projects asset %q: %v", name, err)
	}
	return string(content)
}

func TestZohoProjectsEmbeddedSkill_RegistersAndInstallsContractTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-projects")
	if err != nil {
		t.Fatalf("GetSkill(zoho-projects): %v", err)
	}
	if skill.Name != "Zoho Projects" || skill.Scope != "optional" {
		t.Fatalf("Projects skill metadata = name %q, scope %q; want Zoho Projects optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("Projects skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-projects"}); err != nil {
		t.Fatalf("InstallSelected Projects tree: %v", err)
	}
	for _, name := range zohoProjectsContractAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-projects", name)); err != nil {
			t.Fatalf("installed Projects asset %q: %v", name, err)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_UsesOnlyContractLocalReferences(t *testing.T) {
	linkPattern := regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	for _, name := range zohoProjectsContractAssetNames {
		if path.Ext(name) != ".md" {
			continue
		}
		content := readZohoProjectsContractAsset(t, name)
		for _, match := range linkPattern.FindAllStringSubmatch(content, -1) {
			target := match[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
				continue
			}
			resolved := path.Clean(path.Join(path.Dir(name), target))
			if _, err := fs.Stat(jarvis.SkillsFS, path.Join(zohoProjectsSkillRoot, resolved)); err != nil {
				t.Fatalf("%s local reference %q resolves to missing asset %q: %v", name, target, resolved, err)
			}
		}
	}

	skill := readZohoProjectsContractAsset(t, "SKILL.md")
	for _, excluded := range []string{"current-rest-api.md", "current-rest-operations.csv"} {
		if strings.Contains(skill, excluded) {
			t.Errorf("SKILL.md must not link catalog asset %q before the catalog work unit", excluded)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_FollowsRuntimeContractStructure(t *testing.T) {
	content := readZohoProjectsContractAsset(t, "SKILL.md")
	sections := []string{
		"## Activation Contract",
		"## Hard Rules",
		"## Decision Gates",
		"## Execution Steps",
		"## Output Contract",
		"## References",
	}
	previous := -1
	for _, section := range sections {
		index := strings.Index(content, section)
		if index < 0 {
			t.Fatalf("SKILL.md must contain %q", section)
		}
		if index <= previous {
			t.Fatalf("SKILL.md section %q is out of contract order", section)
		}
		previous = index
	}
	for _, gate := range []string{"Exact Deluge task match", "Verified REST operation", "Contradictory or unavailable evidence", "Destructive operation"} {
		if !strings.Contains(content, gate) {
			t.Errorf("SKILL.md decision gates must contain %q", gate)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_UsesClosedNineTaskCatalog(t *testing.T) {
	content := readZohoProjectsContractAsset(t, "references/deluge-tasks.md")
	tasks := []string{
		"getPortals", "getProjectDetails", "createProject", "getRecords", "getRecordById",
		"create", "update", "associateLogs", "updateAssociateLogs",
	}
	for _, task := range tasks {
		if !strings.Contains(content, "`zoho.projects."+task+"(") {
			t.Errorf("Deluge catalog must include %q", task)
		}
	}
	if got := strings.Count(content, "| `zoho.projects."); got != len(tasks) {
		t.Fatalf("Deluge catalog has %d task rows; want exactly %d", got, len(tasks))
	}
	for _, want := range []string{"named OAuth connection", "exact positional order", "portal only", "portal plus project"} {
		if !strings.Contains(content, want) {
			t.Errorf("Deluge catalog must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesRoutingAndDependencyGates(t *testing.T) {
	content := readZohoProjectsContractAsset(t, "references/routing.md")
	for _, want := range []string{
		"host or target", "actual Deluge output", "zoho-deluge", "requested language", "requested placement",
		"every involved application skill", "WorkDrive", "CRM", "standard Projects functionality",
		"metadata", "permissions", "documents", "attachments", "setup", "automation", "custom modules", "bulk",
		"recommend one and wait",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("routing contract must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesIdentifiersAuthenticationLimitsAndPlans(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  []string
	}{
		{
			name:  "identity distinctions and runtime authority",
			asset: "references/identifiers-and-metadata.md",
			want: []string{
				"portal ID", "portal name", "project ID", "project key", "project name", "ZPUID", "email",
				"field ID", "column_name", "display name", "Projects attachment ID", "WorkDrive upload resource ID",
				"runtime metadata", "exact operation",
			},
		},
		{
			name:  "authentication limits and plan uncertainty",
			asset: "references/authentication-limits-and-plans.md",
			want: []string{
				"Never request or embed secrets", "200 requests per endpoint in two minutes", "ten minutes", "Retry-After",
				"100 Projects integration-task executions in two minutes", "30-minute restriction", "independent",
				"Enterprise-only", "paid plans", "runtime validation",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readZohoProjectsContractAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Errorf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesLifecycleAndUncertaintyBoundaries(t *testing.T) {
	content := readZohoProjectsContractAsset(t, "references/lifecycle-and-uncertainty.md")
	for _, want := range []string{
		"trash", "restore", "activate", "deactivate", "clone", "move", "reorder", "follow", "unfollow",
		"link", "unlink", "associate", "disassociate", "import", "export", "timers", "pins", "default",
		"favourite", "blueprint transitions", "custom-function execution", "bulk jobs", "explicit confirmation",
		"calendar events", "not webhook", "verified", "contradictory", "unavailable", "TBD", "unsupported",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("lifecycle contract must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_RequiresAssumptionsAndExpectedTestOutcomes(t *testing.T) {
	content := readZohoProjectsContractAsset(t, "SKILL.md")
	for _, want := range []string{
		"Assumptions:",
		"Expected test outcomes:",
		"state each inferred or runtime-dependent fact",
		"pair each generated test case with its expected result",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Projects output contract must contain %q", want)
		}
	}
}
