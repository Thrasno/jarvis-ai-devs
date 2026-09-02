package skills

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoPeopleSkillRoot = "embed/skills/zoho-people"

var zohoPeopleCoreAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/deluge-tasks.md",
	"references/entity-identifiers.md",
	"references/authentication-limits-and-plans.md",
	"references/lifecycle-and-webhooks.md",
	"references/uncertainty-and-errors.md",
	"references/sources.md",
}

func readZohoPeopleAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoPeopleSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded People asset %q: %v", name, err)
	}
	return string(content)
}

func assertZohoPeopleAssetContains(t *testing.T, asset string, values ...string) {
	t.Helper()
	content := readZohoPeopleAsset(t, asset)
	for _, value := range values {
		if !strings.Contains(content, value) {
			t.Errorf("%s must contain %q", asset, value)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_RegistersAndInstallsItsCoreTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-people")
	if err != nil {
		t.Fatalf("GetSkill(zoho-people): %v", err)
	}
	if skill.Name != "Zoho People" || skill.Scope != "optional" {
		t.Fatalf("People skill metadata = name %q, scope %q; want Zoho People optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("People skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-people"}); err != nil {
		t.Fatalf("InstallSelected People tree: %v", err)
	}
	for _, name := range zohoPeopleCoreAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-people", name)); err != nil {
			t.Errorf("installed People asset %q: %v", name, err)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_LinksOnlyCoreLocalReferences(t *testing.T) {
	content := readZohoPeopleAsset(t, "SKILL.md")
	for _, name := range zohoPeopleCoreAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Errorf("SKILL.md must link installed core reference %q", name)
		}
	}
	for _, excluded := range []string{
		"references/rest-v1-v2-catalog.md",
		"references/rest-v3-catalog.md",
	} {
		if strings.Contains(content, excluded) {
			t.Errorf("SKILL.md must not link deferred catalog %q", excluded)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_StatesActivationRoutingAndSafetyContracts(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{
			asset: "SKILL.md",
			want: []string{
				"host or target", "zoho-deluge", "standard People functionality", "runtime metadata",
				"contradictory", "unavailable", "TBD", "explicit confirmation", "bulk or destructive",
				"minimize personal data", "placeholders", "logs",
			},
		},
		{
			asset: "references/routing.md",
			want: []string{
				"host", "target", "execution context", "API family", "operation", "plan",
				"every involved application skill", "equally optimal", "wait for selection",
			},
		},
		{
			asset: "references/uncertainty-and-errors.md",
			want:  []string{"only missing facts", "fail closed", "safe clarification", "unsupported", "Do not invent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			assertZohoPeopleAssetContains(t, tt.asset, tt.want...)
		})
	}
}

func TestZohoPeopleEmbeddedSkill_UsesExactlyFourNativeDelugeTasks(t *testing.T) {
	content := readZohoPeopleAsset(t, "references/deluge-tasks.md")
	allowlist := []string{
		"zoho.people.getRecords(form_name, [from_index], [count], [search_criteria], [connection])",
		"zoho.people.create(form_name, record_values, [connection])",
		"zoho.people.getRecordById(form_name, record_id, [connection])",
		"zoho.people.update(form_name, new_values, [connection])",
	}
	for _, task := range allowlist {
		if !strings.Contains(content, "`"+task+"`") {
			t.Errorf("Deluge catalog must include %q", task)
		}
	}
	if got := strings.Count(content, "| `zoho.people."); got != len(allowlist) {
		t.Fatalf("Deluge catalog has %d task rows; want exactly %d", got, len(allowlist))
	}
	assertZohoPeopleAssetContains(t, "references/deluge-tasks.md",
		"maximum 200 records", "positional order", "searchField", "field link name",
		"form and field label names", "People record ID", "recordid", "named OAuth connections",
		"not applicable in Creator", "mandatory in Cliq", "one external call",
		"specialized leave, attendance, time, cases, LMS, compensation, performance, survey, files, webhook, or HR-process operations",
	)
}

func TestZohoPeopleEmbeddedSkill_PreservesEntityAndIdentifierBoundaries(t *testing.T) {
	assertZohoPeopleAssetContains(t, "references/entity-identifiers.md",
		"form label name", "form link name", "field label name", "field link name", "display name",
		"record ID", "employee ID", "email", "user ID", "erecno", "Zoho_ID", "pkId", "recordid",
		"People Time Tracker", "not Zoho Projects IDs", "General People Files", "LMS Files",
		"Specialized domains must not route through generic form CRUD",
	)
}

func TestZohoPeopleEmbeddedSkill_RequiresSafeAuthenticationLimitsPlansAndLifecycle(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{
			asset: "references/authentication-limits-and-plans.md",
			want: []string{
				"OAuth 2.0 only", "named connections", "Never request or embed", "api_domain",
				"US", "AU", "EU", "IN", "CN", "JP", "one hour", "one-time", "until revoked",
				"Essential HR", "5,000", "Professional", "10,000", "Premium", "15,000", "Enterprise", "25,000",
				"endpoint thresholds", "lock periods", "TBD", "runtime validation",
			},
		},
		{
			asset: "references/lifecycle-and-webhooks.md",
			want: []string{
				"publish", "unpublish", "approve", "cancel", "pause", "resume", "enroll", "unenroll",
				"enable", "disable", "mark", "reminder", "product configuration", "not verified REST CRUD",
				"management endpoint", "payload", "retries", "signing", "TBD", "fail closed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			assertZohoPeopleAssetContains(t, tt.asset, tt.want...)
		})
	}
}

func TestZohoPeopleEmbeddedSkill_CitesOfficialSourcesAndSnapshot(t *testing.T) {
	assertZohoPeopleAssetContains(t, "references/sources.md",
		"Snapshot date: 2026-09-01", "Menu index", "v1/v2 overview", "v3 overview",
		"OAuth", "Scopes", "Limits", "Deluge People tasks", "Webhooks product help",
		"authenticated runtime metadata", "Missing evidence must never be inferred",
	)
}
