package skills

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoCreatorSkillRoot = "embed/skills/zoho-creator"

var zohoCreatorAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/native-deluge.md",
	"references/integration-tasks-v2.md",
	"references/rest-api.md",
	"references/workflows-and-subforms.md",
	"references/identity-auth-and-environments.md",
	"references/uncertainty-and-errors.md",
	"references/sources.md",
}

func readZohoCreatorAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoCreatorSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded Creator asset %q: %v", name, err)
	}
	return string(content)
}

func countZohoCreatorRows(content, prefix string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

func assertZohoCreatorContains(t *testing.T, asset, content string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Fatalf("%s must contain %q", asset, want)
		}
	}
}

func TestZohoCreatorEmbeddedSkill_RegistersAndInstallsItsCompleteTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-creator")
	if err != nil {
		t.Fatalf("GetSkill(zoho-creator): %v", err)
	}
	if skill.Name != "Zoho Creator" || skill.Scope != "optional" {
		t.Fatalf("Creator skill metadata = name %q, scope %q; want Zoho Creator optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("Creator skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-creator"}); err != nil {
		t.Fatalf("InstallSelected Creator tree: %v", err)
	}
	for _, name := range zohoCreatorAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-creator", name)); err != nil {
			t.Fatalf("installed Creator asset %q: %v", name, err)
		}
	}
}

func TestZohoCreatorEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	content := readZohoCreatorAsset(t, "SKILL.md")
	for _, name := range zohoCreatorAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Fatalf("SKILL.md must link installed reference %q", name)
		}
	}
}

func TestZohoCreatorEmbeddedSkill_EnforcesOwnershipAndRouting(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  []string
	}{
		{
			name:  "Creator owns entities and Deluge owns grammar",
			asset: "SKILL.md",
			want:  []string{"zoho-creator", "zoho-deluge", "entities, applicability, execution context, workflows, permissions, events, and routing semantics"},
		},
		{
			name:  "forms write while reports project records",
			asset: "references/routing.md",
			want:  []string{"Forms are schema and write units", "Reports are projections", "form_link_name", "report_link_name"},
		},
		{
			name:  "surface precedence preserves operation boundaries",
			asset: "references/routing.md",
			want:  []string{"same application", "native Creator statements", "exact five-task allowlist", "REST", "delete", "v2.1-specific behavior", "metadata", "files", "publish", "bulk read"},
		},
		{
			name:  "delete never invents an integration task",
			asset: "references/routing.md",
			want:  []string{"native `delete from`", "REST DELETE", "unsupported"},
		},
		{
			name:  "writes require confirmation",
			asset: "SKILL.md",
			want:  []string{"explicit confirmation", "create, update, delete, upload, publish, or metadata-changing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readZohoCreatorAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoCreatorEmbeddedSkill_CatalogsNativeStatements(t *testing.T) {
	const asset = "references/native-deluge.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"insert into <form_link_name>",
		"<form_link_name>[<criteria>]",
		"<record>.<field_link_name> = <expression>",
		"delete from <form_link_name>[<criteria>]",
		"input.<subform>.insert(<collection>)",
		"input.<subform_link_name>.clear()",
	)
	if got := countZohoCreatorRows(content, "| `"); got != 6 {
		t.Fatalf("native statement catalog has %d rows; want exactly 6", got)
	}
}

func TestZohoCreatorEmbeddedSkill_CatalogsExactIntegrationTasks(t *testing.T) {
	const asset = "references/integration-tasks-v2.md"
	content := readZohoCreatorAsset(t, asset)
	allowlist := []string{
		"zoho.creator.getRecords(owner_name, app_link_name, report_link_name, criteria, from_index, limit, connection)",
		"zoho.creator.getRecordById(owner_name, app_link_name, report_link_name, record_id, connection)",
		"zoho.creator.createRecord(owner_name, app_link_name, form_link_name, input_values, other_params, connection)",
		"zoho.creator.updateRecords(owner_name, app_link_name, report_link_name, criteria, new_input_values, other_api_params, connection)",
		"zoho.creator.updateRecord(owner_name, app_link_name, report_link_name, record_id, new_input_values, other_api_params, connection)",
	}
	for _, signature := range allowlist {
		assertZohoCreatorContains(t, asset, content, "`"+signature+"`")
	}
	if got := countZohoCreatorRows(content, "| `zoho.creator."); got != len(allowlist) {
		t.Fatalf("integration-task catalog has %d task rows; want exactly %d", got, len(allowlist))
	}
	assertZohoCreatorContains(t, asset, content,
		"There is no Creator delete integration task",
		"v2 wrappers",
		"must not inherit v2.1-only controls",
		"backend API request",
		"maximum 200",
	)
}

func TestZohoCreatorEmbeddedSkill_CatalogsRESTV2DataOperations(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"Replace `{version}` with `v2` or `v2.1`",
		"## Data — 7 operations",
		"POST /creator/{version}/data/{owner}/{app}/form/{form}",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}",
		"PATCH /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"PATCH /creator/{version}/data/{owner}/{app}/report/{report}",
		"DELETE /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"DELETE /creator/{version}/data/{owner}/{app}/report/{report}",
		"Do not apply these controls to v2 pages or v2-backed integration tasks",
	)
}

func TestZohoCreatorEmbeddedSkill_CatalogsRESTV21DataOperations(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"Replace `{version}` with `v2` or `v2.1`",
		"## Data — 7 operations",
		"POST /creator/{version}/data/{owner}/{app}/form/{form}",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}",
		"PATCH /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"PATCH /creator/{version}/data/{owner}/{app}/report/{report}",
		"DELETE /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}",
		"DELETE /creator/{version}/data/{owner}/{app}/report/{report}",
		"v2.1 record reads add `record_cursor`",
		"field-selection controls",
		"JSON/CSV output",
	)
}

func TestZohoCreatorEmbeddedSkill_CatalogsPublishOperations(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"## Publish — 3 operations",
		"POST /creator/{version}/publish/{owner}/{app}/form/{form}",
		"GET /creator/{version}/publish/{owner}/{app}/report/{report}/{record_ID}",
		"GET /creator/{version}/publish/{owner}/{app}/report/{report}",
		"Publish APIs are production-only",
		"mandatory private link",
	)
}

func TestZohoCreatorEmbeddedSkill_CatalogsFileOperationsAndBlocksContradictorySubformDownload(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"## File — 3 operations",
		"POST /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}/{field_link_name}/upload",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}/{field_link_name}/download",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}/{subform_link_name}.{field_link_name}/{subform_record_ID}/download",
		"GET /creator/{version}/data/{owner}/{app}/report/{report}/{subform_link_name}/{field_link_name}/{subform_record_ID}/download",
		"operation page",
		"OpenAPI",
		"must fail closed",
		"Do not select either documented shape",
	)
	if strings.Contains(content, "{subform-and-field}") {
		t.Fatal("REST file contract must not invent a combined {subform-and-field} segment")
	}
}

func TestZohoCreatorEmbeddedSkill_CatalogsMetadataOperations(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"## Metadata — 7 operations",
		"GET /creator/{version}/meta/{owner}/{app}/form/{form}/fields",
		"GET /creator/{version}/meta/{owner}/{app}/forms",
		"GET /creator/{version}/meta/{owner}/{app}/reports",
		"GET /creator/{version}/meta/{owner}/{app}/pages",
		"GET /creator/{version}/meta/{owner}/{app}/sections",
		"GET /creator/{version}/meta/applications",
		"GET /creator/{version}/meta/{account_owner_name}/applications",
	)
}

func TestZohoCreatorEmbeddedSkill_CatalogsBulkReadAndBlocksBulkInsert(t *testing.T) {
	const asset = "references/rest-api.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"## Bulk read — 3 operations",
		"POST /creator/{version}/bulk/{owner}/{app}/report/{report}/read",
		"GET /creator/{version}/bulk/{owner}/{app}/report/{report}/read/{job_ID}",
		"GET /creator/{version}/bulk/{owner}/{app}/report/{report}/read/{job_ID}/result",
		"Bulk insert is blocked",
		"expose only these three bulk-read operations",
	)
}

func TestZohoCreatorEmbeddedSkill_RESTManifestKeepsExactOperationCount(t *testing.T) {
	content := readZohoCreatorAsset(t, "references/rest-api.md")
	if got := countZohoCreatorRows(content, "| `"); got != 23 {
		t.Fatalf("REST operation manifest has %d rows; want exactly 23", got)
	}
}

func TestZohoCreatorEmbeddedSkill_PreservesWorkflowEffects(t *testing.T) {
	const asset = "references/workflows-and-subforms.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"On Validate and On Success scripts do not execute",
		"v2.1 add, update, delete, and upload execute associated workflows by default",
		"add and upload retain blueprints and approvals",
		"update and delete retain blueprints",
	)
}

func TestZohoCreatorEmbeddedSkill_PreservesSubformBehavior(t *testing.T) {
	const asset = "references/workflows-and-subforms.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"main form's subform constructor",
		"input.<subform>.insert(...)",
		"only in supported workflow contexts",
		"custom-sorted subforms",
		"input.<subform_link_name>.clear()",
		"removes every row, not selected rows",
		"delinks existing-form child records",
		"does not delete those child records unless they are explicitly deleted",
		"record IDs and subform record IDs distinct",
		"contradictory subform file-download path blocked",
	)
}

func TestZohoCreatorEmbeddedSkill_PreservesIdentityAuthenticationAndEnvironmentSemantics(t *testing.T) {
	const asset = "references/identity-auth-and-environments.md"
	content := readZohoCreatorAsset(t, asset)
	assertZohoCreatorContains(t, asset, content,
		"environment: development|stage",
		"production is the default",
		"Publish APIs are production-only",
		"api_domain returned by OAuth",
		"US, EU, IN, AU, JP, CA, SA, CN, and UAE",
		"Never expose OAuth tokens or published private links",
		"API Access permission",
	)
}

func TestZohoCreatorEmbeddedSkill_PreservesAdvancedAnalyticsConnectorInvariant(t *testing.T) {
	content := readZohoCreatorAsset(t, "references/routing.md")
	for _, want := range []string{
		"Zoho Creator Advanced Analytics",
		"automatically synchronizes Creator data for Analytics reports, dashboards, sharing, publishing, and embedding",
		"Prefer it for Creator reporting and dashboard use cases only when its verified capabilities and required cadence are sufficient",
		"Never describe the connector as real-time",
		"exact cadence, entity coverage, plan gate, conflict handling, and failure/recovery semantics remain unresolved",
		"immediate event-driven row mutations",
		"bulk synchronization, import, or export",
		"load `zoho-analytics` and follow its routing contract",
		"unsupported when no verified surface satisfies the request",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Creator Advanced Analytics invariant must contain %q", want)
		}
	}
}

func TestZohoCreatorEmbeddedSkill_BoundsUncertaintyAndCapabilities(t *testing.T) {
	content := readZohoCreatorAsset(t, "references/uncertainty-and-errors.md")
	for _, want := range []string{
		"Unknown plan gates, quotas, and costs",
		"warning and runtime validation",
		"Custom API generation remains unavailable",
		"Bulk insert remains excluded",
		"subform file-download route remains blocked",
		"Do not invent",
		"only missing facts that change routing or prevent safe generation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("uncertainty contract must contain %q", want)
		}
	}
}
