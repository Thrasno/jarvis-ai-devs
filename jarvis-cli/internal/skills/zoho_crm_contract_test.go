package skills

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoCRMSkillRoot = "embed/skills/zoho-crm"

func readZohoCRMAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoCRMSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded CRM asset %q: %v", name, err)
	}
	return string(content)
}

func TestZohoCRMEmbeddedSkill_RegistersAndInstallsItsCompleteTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-crm")
	if err != nil {
		t.Fatalf("GetSkill(zoho-crm): %v", err)
	}
	if skill.Name != "Zoho CRM" || skill.Scope != "optional" {
		t.Fatalf("CRM skill metadata = name %q, scope %q; want Zoho CRM optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("CRM skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-crm"}); err != nil {
		t.Fatalf("InstallSelected CRM tree: %v", err)
	}
	for _, name := range zohoCRMAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-crm", name)); err != nil {
			t.Fatalf("installed CRM asset %q: %v", name, err)
		}
	}
}

func TestZohoCRMEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	content := readZohoCRMAsset(t, "SKILL.md")
	for _, name := range zohoCRMAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Fatalf("SKILL.md must link installed reference %q", name)
		}
	}
}

var zohoCRMAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/execution-contexts.md",
	"references/deluge-tasks-v8.md",
	"references/rest-v8.md",
	"references/client-script.md",
	"references/metadata-and-prerequisites.md",
	"references/authentication.md",
	"references/standard-capabilities.md",
	"references/uncertainty-and-errors.md",
	"references/sources.md",
	"references/zoho-crm-standard-modules.md",
	"references/zoho-crm-standard-fields.md",
}

func TestZohoCRMEmbeddedSkill_StatesActivationPlacementAndRoutingContracts(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  []string
	}{
		{"CRM Deluge composes the language skill and Deluge output", "references/routing.md", []string{"zoho-crm", "zoho-deluge", "[name].deluge"}},
		{"cross application Deluge composes every application skill", "references/routing.md", []string{"every involved application skill", "zoho-deluge", "[name].deluge"}},
		{"Client Script stays JavaScript without Deluge", "references/client-script.md", []string{"JavaScript", "[name].js", "MUST NOT load `zoho-deluge`"}},
		{"external runtimes keep requested language and placement", "references/routing.md", []string{"requested language", "requested placement", "not Deluge"}},
		{"allowlist misses evaluate verified alternatives", "references/routing.md", []string{"standard CRM configuration", "Client Script/ZDK", "REST V8", "COQL", "metadata/bulk APIs", "manual workaround"}},
		{"legacy modifications ask for migration choice and API names", "references/deluge-tasks-v8.md", []string{"module and field API names", "migrate to V8", "preserve legacy behavior", "New code uses `zoho.crm.v8.*` only"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readZohoCRMAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoCRMEmbeddedSkill_UsesClosedV8CatalogAndSafeResponseBoundaries(t *testing.T) {
	content := readZohoCRMAsset(t, "references/deluge-tasks-v8.md")
	allowlist := []string{
		"createRecord", "getRecords", "searchRecords", "getRecordById", "updateRecord",
		"bulkCreate", "bulkUpdate", "getRelatedRecords", "updateRelatedRecord", "convertLead",
		"upsert", "attachFile", "getFields",
	}
	for _, task := range allowlist {
		if !strings.Contains(content, "`"+task+"`") {
			t.Fatalf("V8 catalog must include %q", task)
		}
	}
	if strings.Count(content, "| `") != len(allowlist) {
		t.Fatalf("V8 catalog has %d task rows; want exactly %d", strings.Count(content, "| `"), len(allowlist))
	}
	for _, want := range []string{
		"toJsonList()", "for each record in response", "getRelatedRecords`, `bulkCreate`, and `bulkUpdate` opaque", "Never invent a REST-style `data` wrapper",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("response contract must contain %q", want)
		}
	}
}

func TestZohoCRMEmbeddedSkill_CatalogsAreRecognitionOnlyAndRuntimeAuthoritative(t *testing.T) {
	modules := readZohoCRMAsset(t, "references/zoho-crm-standard-modules.md")
	moduleRows := 0
	for _, line := range strings.Split(modules, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| Display Name") && !strings.HasPrefix(line, "|---") {
			moduleRows++
		}
	}
	if got := moduleRows; got != 21 {
		t.Fatalf("module catalog has %d rows; want 21", got)
	}
	for _, want := range []string{"recognition", "Runtime metadata remains authoritative", "not a write-safety"} {
		if !strings.Contains(modules, want) {
			t.Fatalf("module catalog must contain %q", want)
		}
	}

	fields := readZohoCRMAsset(t, "references/zoho-crm-standard-fields.md")
	if got := strings.Count(fields, "## "); got != 21 {
		t.Fatalf("field catalog has %d sections; want 21", got)
	}
	rows := 0
	for _, line := range strings.Split(fields, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| Field Display Name") && !strings.HasPrefix(line, "|---") {
			rows++
		}
	}
	if rows != 609 {
		t.Fatalf("field catalog has %d rows; want 609", rows)
	}
	if !strings.Contains(fields, "Runtime metadata remains authoritative") {
		t.Fatal("field catalog must defer organization facts to runtime metadata")
	}
}

func TestZohoCRMEmbeddedSkill_PreservesSafetyAndBoundedUncertainty(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{"references/authentication.md", []string{"conpas_crm", "exact operation", "Never request or embed", "secure deployment"}},
		{"references/uncertainty-and-errors.md", []string{"only missing facts", "one valid path", "equally optimal", "wait for selection", "schedule arguments", "implicit custom-button arguments", "validation signatures", "Quick Create", "Function API endpoint templates", "non-CRM applications"}},
		{"references/rest-v8.md", []string{"Avoid unnecessary parallelism", "must not include quotas", "credit formulas", "capacity estimates", "numeric timeouts", "exact concurrency thresholds"}},
	}
	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoCRMAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  string
	}{
		{
			name:  "allowlisted V8 operations do not ask for an API version",
			asset: "references/routing.md",
			want:  "An allowlisted V8 task needs no API-version question.",
		},
		{
			name:  "standard CRM alternatives remain non-blocking advice",
			asset: "references/standard-capabilities.md",
			want:  "I will continue with the requested custom implementation unless you choose the standard path.",
		},
		{
			name:  "equally optimal paths include a recommendation before waiting",
			asset: "references/routing.md",
			want:  "When paths are equally optimal, recommend one and wait for selection.",
		},
		{
			name:  "excluded facts offer a safe clarification or alternative",
			asset: "references/uncertainty-and-errors.md",
			want:  "offer a safe clarification, target configuration, or verified alternative.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if content := readZohoCRMAsset(t, tt.asset); !strings.Contains(content, tt.want) {
				t.Fatalf("%s must contain %q", tt.asset, tt.want)
			}
		})
	}
}
