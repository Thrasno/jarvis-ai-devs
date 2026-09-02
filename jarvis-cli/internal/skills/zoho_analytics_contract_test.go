package skills

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoAnalyticsSkillRoot = "embed/skills/zoho-analytics"

var zohoAnalyticsAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/entities-and-identifiers.md",
	"references/deluge-tasks.md",
	"references/async-and-sql.md",
	"references/authentication-and-limits.md",
	"references/uncertainty-and-errors.md",
	"references/sources.md",
}

func readZohoAnalyticsAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoAnalyticsSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded Analytics asset %q: %v", name, err)
	}
	return string(content)
}

func TestZohoAnalyticsEmbeddedSkill_RegistersAndInstallsItsRoutingTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-analytics")
	if err != nil {
		t.Fatalf("GetSkill(zoho-analytics): %v", err)
	}
	if skill.Name != "Zoho Analytics" || skill.Scope != "optional" {
		t.Fatalf("Analytics skill metadata = name %q, scope %q; want Zoho Analytics optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("Analytics skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-analytics"}); err != nil {
		t.Fatalf("InstallSelected Analytics tree: %v", err)
	}
	for _, name := range zohoAnalyticsAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-analytics", name)); err != nil {
			t.Fatalf("installed Analytics asset %q: %v", name, err)
		}
	}
}

func TestZohoAnalyticsEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	content := readZohoAnalyticsAsset(t, "SKILL.md")
	for _, name := range zohoAnalyticsAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Fatalf("SKILL.md must link installed reference %q", name)
		}
	}
	if strings.Contains(content, "references/rest-v2-catalog.md") {
		t.Fatal("routing contract snapshot must not link the deferred REST v2 catalog")
	}
}

func TestZohoAnalyticsEmbeddedSkill_RejectsInventedDelugeReadTasks(t *testing.T) {
	content := readZohoAnalyticsAsset(t, "references/deluge-tasks.md")
	for _, forbidden := range []string{"zoho.reports.get", "zoho.reports.read", "zoho.reports.export"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("Deluge allowlist must not invent Analytics read task %q", forbidden)
		}
	}
	for _, operation := range []string{"| Create row |", "| Update rows |", "| Delete rows |"} {
		if strings.Count(content, operation) != 1 {
			t.Fatalf("Deluge allowlist must contain exactly one %q row", operation)
		}
	}
}

func TestZohoAnalyticsEmbeddedSkill_PreservesRoutingAndCapabilityBoundaries(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{
			asset: "references/routing.md",
			want: []string{
				"Zoho Creator Advanced Analytics",
				"verified capabilities and required cadence are sufficient",
				"immediate event-driven row mutations",
				"bulk synchronisation/import/export",
				"Never describe the standard connector as real-time",
				"source host", "target organization/workspace", "identity form", "connection", "scope", "units", "code placement",
			},
		},
		{
			asset: "references/deluge-tasks.md",
			want: []string{
				"zoho.reports.createRow(database_name, table_name, data_map, connection)",
				"zoho.reports.updateData(database_name, table_name, data_map, criteria, connection)",
				"zoho.reports.deleteRow(database_name, table_name, criteria, connection)",
				"There is no Analytics read integration task",
				"verified Deluge-capable host",
			},
		},
		{
			asset: "references/entities-and-identifiers.md",
			want:  []string{"orgId", "workspaceId", "viewId", "columnId", "jobId", "Discover IDs through metadata"},
		},
		{
			asset: "references/async-and-sql.md",
			want: []string{
				"poll", "download", "callbackUrl", "job notification", "not evidence of a general Analytics event/webhook subsystem",
				"CONFIG.sqlQuery", "SQL `SELECT`", "persistent Query Table", "CloudSQL JDBC", "outside this routing and execution-contract snapshot",
			},
		},
		{
			asset: "references/authentication-and-limits.md",
			want: []string{
				"OAuth 2.0", "ZANALYTICS-ORGID", "analyticsapi.zohocloud.ca", "Free 1,000", "Enterprise 100,000",
				"DML 100/minute", "bulk 40/minute", "metadata 60/minute", "overall 100/minute", "runtime validation",
			},
		},
		{
			asset: "references/uncertainty-and-errors.md",
			want:  []string{"write confirmation", "unknown plan gates", "unlisted costs", "contradictory", "fail closed", "unsupported"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoAnalyticsAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}
