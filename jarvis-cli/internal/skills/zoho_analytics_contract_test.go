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
	"references/rest-v2-catalog.md",
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

func TestZohoAnalyticsEmbeddedSkill_RegistersAndInstallsItsCompleteTree(t *testing.T) {
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
}

func TestZohoAnalyticsEmbeddedSkill_FreezesCompleteRESTV2Catalog(t *testing.T) {
	content := readZohoAnalyticsAsset(t, "references/rest-v2-catalog.md")
	families := map[string]int{
		"Data":                      3,
		"Bulk":                      12,
		"Modeling":                  65,
		"Metadata":                  47,
		"Sharing and collaboration": 17,
		"Embed":                     15,
		"User management":           17,
	}
	for family, count := range families {
		heading := "## " + family + " — "
		start := strings.Index(content, heading)
		if start < 0 {
			t.Fatalf("REST catalog missing family %q", family)
		}
		section := content[start:]
		if next := strings.Index(section[len(heading):], "\n## "); next >= 0 {
			section = section[:len(heading)+next]
		}
		rows := 0
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "| `GET` |") || strings.HasPrefix(line, "| `POST` |") || strings.HasPrefix(line, "| `PUT` |") || strings.HasPrefix(line, "| `DELETE` |") || strings.HasPrefix(line, "| `PATCH` |") {
				rows++
			}
		}
		if rows != count {
			t.Fatalf("%s catalog has %d operations; want %d", family, rows, count)
		}
	}

	seen := make(map[string]struct{})
	for _, line := range strings.Split(content, "\n") {
		if !(strings.HasPrefix(line, "| `GET` |") || strings.HasPrefix(line, "| `POST` |") || strings.HasPrefix(line, "| `PUT` |") || strings.HasPrefix(line, "| `DELETE` |") || strings.HasPrefix(line, "| `PATCH` |")) {
			continue
		}
		fields := strings.Split(line, " | ")
		if len(fields) < 5 {
			t.Fatalf("malformed operation row: %q", line)
		}
		key := fields[0] + " " + fields[1]
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate method/path operation %q", key)
		}
		seen[key] = struct{}{}
		if !strings.Contains(fields[4], "ZohoAnalytics.") {
			t.Fatalf("operation %q has no exact OpenAPI scope: %q", key, fields[4])
		}
	}
	if len(seen) != 176 {
		t.Fatalf("REST catalog has %d unique method/path operations; want 176", len(seen))
	}

	for _, want := range []string{
		"176 active verified REST v2 operations",
		"import-job scope",
		"Save As scope",
		"nine group/permission paths",
		"mutation scopes",
		"metadata.updatde",
		"`/resourceDetails`",
		"`/resources`",
		"fail closed",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("REST catalog must preserve %q", want)
		}
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
				"CONFIG.sqlQuery", "SQL `SELECT`", "persistent Query Table", "CloudSQL JDBC", "outside V0 routing, generation, and catalog scope",
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
