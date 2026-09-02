package skills

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoBooksSkillRoot = "embed/skills/zoho-books"

var zohoBooksAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/integration-tasks.md",
	"references/rest-v3.md",
	"references/standard-capabilities.md",
	"references/standard-resources-and-fields.md",
	"references/runtime-safety-and-output.md",
	"references/sources.md",
}

func readZohoBooksAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoBooksSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded Books asset %q: %v", name, err)
	}
	return string(content)
}

func TestZohoBooksEmbeddedSkill_RegistersAndInstallsCompleteTree(t *testing.T) {
	skill, err := GetSkill(jarvis.SkillsFS, "zoho-books")
	if err != nil {
		t.Fatalf("GetSkill(zoho-books): %v", err)
	}
	if skill.Name != "Zoho Books" || skill.Scope != "optional" {
		t.Fatalf("Books skill metadata = name %q, scope %q; want Zoho Books optional", skill.Name, skill.Scope)
	}
	if !strings.Contains(skill.Description, "Trigger:") {
		t.Fatalf("Books skill description must declare its trigger: %q", skill.Description)
	}

	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-books"}); err != nil {
		t.Fatalf("InstallSelected Books tree: %v", err)
	}
	for _, name := range zohoBooksAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-books", name)); err != nil {
			t.Fatalf("installed Books asset %q: %v", name, err)
		}
	}
}

func TestZohoBooksEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	content := readZohoBooksAsset(t, "SKILL.md")
	for _, name := range zohoBooksAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Fatalf("SKILL.md must link installed reference %q", name)
		}
	}
}

func TestZohoBooksEmbeddedSkill_StatesActivationCompositionAndRouting(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  []string
	}{
		{"Books owns application facts in every role", "references/routing.md", []string{"host application", "target application", "cross-application participant"}},
		{"application and language ownership stay orthogonal", "references/routing.md", []string{"orthogonal", "zoho-books", "zoho-deluge", "requested language", "requested placement"}},
		{"standard advice comes first without blocking code", "references/routing.md", []string{"standard Books capability", "advisory", "non-blocking", "explicit code request"}},
		{"compatible tasks precede REST without automatic fallback", "references/routing.md", []string{"compatible Integration Task", "before REST v3", "never selects REST automatically", "another verified surface", "manual", "unsupported"}},
		{"equally optimal paths require human selection", "references/routing.md", []string{"equally optimal", "recommend one", "wait for selection"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readZohoBooksAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoBooksEmbeddedSkill_ClosesIndependentCatalogsAndProvenance(t *testing.T) {
	rest := readZohoBooksAsset(t, "references/rest-v3.md")
	for _, want := range []string{
		"849 operations", "42 resources", "GET 263", "POST 338", "PUT 131", "DELETE 117",
		"zero operation omissions", "c6a841bbc81ef882c64b1f2ad4761e350faefba1db7fb36a32daf7112b647559",
		"841 require `organization_id`", "8 omit it", "per-operation prerequisite",
	} {
		if !strings.Contains(rest, want) {
			t.Fatalf("REST catalog must contain %q", want)
		}
	}

	tasks := readZohoBooksAsset(t, "references/integration-tasks.md")
	if got := strings.Count(tasks, "| `zoho.books."); got != 7 {
		t.Fatalf("Integration Task catalog has %d rows; want 7", got)
	}
	for _, want := range []string{"books_connection", "connection", "getRecordsByID", "getRecordsById", "do not claim case-insensitivity"} {
		if !strings.Contains(tasks, want) {
			t.Fatalf("Integration Task catalog must contain %q", want)
		}
	}

	capabilities := readZohoBooksAsset(t, "references/standard-capabilities.md")
	if got := strings.Count(capabilities, "| `"); got != 20 {
		t.Fatalf("standard-capability catalog has %d rows; want 20", got)
	}
	for _, want := range []string{"recognition-only", "advisory", "non-blocking", "Runtime"} {
		if !strings.Contains(capabilities, want) {
			t.Fatalf("standard-capability catalog must contain %q", want)
		}
	}

	structures := readZohoBooksAsset(t, "references/standard-resources-and-fields.md")
	if got := strings.Count(structures, "| `"); got != 42 {
		t.Fatalf("standard-resource catalog has %d rows; want 42", got)
	}
	for _, want := range []string{
		"4,671 component schemas", "2,527 inline roots", "7,198 total roots", "20,987",
		"zero unresolved references", "zero cycles", "zero malformed records", "1,084 valid schema markers",
		"zero reconciliation defects", "recognition-only", "Runtime organization metadata", "no custom or tenant-specific fields",
	} {
		if !strings.Contains(structures, want) {
			t.Fatalf("resource and field catalog must contain %q", want)
		}
	}
}

func TestZohoBooksEmbeddedSkill_PreservesPrerequisitesVersionsSafetyAndUnknowns(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{"references/rest-v3.md", []string{"New REST code uses Books API v3", "legacy REST", "migrate", "preserve", "Never change API versions without explicit confirmation", "existing version and calls"}},
		{"references/runtime-safety-and-output.md", []string{"conpas_books", "Never request, embed, or expose secrets", "metadata", "settings", "plan", "region", "permissions", "providers"}},
		{"references/runtime-safety-and-output.md", []string{"get_delivery_challan_attachment", "TBD", "must not fabricate", "destructive or financial code", "without redundant confirmation", "Live execution is excluded", "impact warning", "explicit confirmation"}},
		{"references/runtime-safety-and-output.md", []string{"selected language and surface", "placement/configuration", "argument mapping", "named connection", "assumptions", "standard-capability advisory", "test cases", "expected outcomes"}},
		{"references/runtime-safety-and-output.md", []string{"quotas", "credits", "concurrency", "capacity", "timeouts", "capability availability"}},
		{"references/routing.md", []string{"Inventory", "Expense", "Billing", "Payments", "Payroll", "outside `zoho-books`"}},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoBooksAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}
