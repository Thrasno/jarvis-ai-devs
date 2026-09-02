package skills

import (
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoProjectsSkillRoot = "embed/skills/zoho-projects"

func readZohoProjectsAsset(t *testing.T, name string) string {
	t.Helper()
	content, err := fs.ReadFile(jarvis.SkillsFS, zohoProjectsSkillRoot+"/"+name)
	if err != nil {
		t.Fatalf("read embedded Projects asset %q: %v", name, err)
	}
	return string(content)
}

var zohoProjectsAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/deluge-tasks.md",
	"references/current-rest-api.md",
	"references/current-rest-operations.csv",
	"references/identifiers-and-metadata.md",
	"references/authentication-limits-and-plans.md",
	"references/lifecycle-and-uncertainty.md",
	"references/sources.md",
}

func TestZohoProjectsEmbeddedSkill_RegistersAndInstallsItsCompleteTree(t *testing.T) {
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
	for _, name := range zohoProjectsAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-projects", name)); err != nil {
			t.Fatalf("installed Projects asset %q: %v", name, err)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	linkPattern := regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	for _, name := range zohoProjectsAssetNames {
		if path.Ext(name) != ".md" {
			continue
		}
		content := readZohoProjectsAsset(t, name)
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
}

func TestZohoProjectsEmbeddedSkill_FollowsRuntimeSkillStructure(t *testing.T) {
	content := readZohoProjectsAsset(t, "SKILL.md")
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
	for _, gate := range []string{"Exact Deluge task match", "Verified REST catalog row", "Contradictory or unavailable evidence", "Destructive operation"} {
		if !strings.Contains(content, gate) {
			t.Fatalf("SKILL.md decision gates must contain %q", gate)
		}
	}
	for _, step := range []string{"Identify the host", "Check standard Projects functionality", "Select one catalog record", "Return the output contract"} {
		if !strings.Contains(content, step) {
			t.Fatalf("SKILL.md execution steps must contain %q", step)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_StopsOnTBDMetadata(t *testing.T) {
	content := readZohoProjectsAsset(t, "SKILL.md")
	for _, want := range []string{
		"Stop generation and execution until runtime authority verifies every `TBD` metadata value.",
		"If runtime authority cannot verify the missing metadata, return unsupported.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("SKILL.md TBD gate must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesRoutingAndProgressiveContext(t *testing.T) {
	tests := []struct {
		name  string
		asset string
		want  []string
	}{
		{"Projects activation and Deluge composition", "references/routing.md", []string{"host or target", "actual Deluge output", "zoho-deluge", "requested language", "requested placement"}},
		{"cross-product composition", "references/routing.md", []string{"every involved application skill", "WorkDrive", "CRM"}},
		{"progressive portal and project prerequisites", "references/deluge-tasks.md", []string{"getPortals", "neither portal nor project", "portal only", "portal plus project"}},
		{"operation-specific REST routing", "references/current-rest-api.md", []string{"https://projectsapi.zoho.com", "/api/v3", "/api/v3.1", "Never rewrite", "contradictory"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := readZohoProjectsAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoProjectsEmbeddedSkill_UsesClosedNineTaskCatalog(t *testing.T) {
	content := readZohoProjectsAsset(t, "references/deluge-tasks.md")
	tasks := []string{
		"getPortals", "getProjectDetails", "createProject", "getRecords", "getRecordById",
		"create", "update", "associateLogs", "updateAssociateLogs",
	}
	for _, task := range tasks {
		if !strings.Contains(content, "`zoho.projects."+task+"(") {
			t.Fatalf("Deluge catalog must include %q", task)
		}
	}
	if got := strings.Count(content, "| `zoho.projects."); got != len(tasks) {
		t.Fatalf("Deluge catalog has %d task rows; want exactly %d", got, len(tasks))
	}
	for _, want := range []string{"milestones", "taskLists", "tasks", "bugs", "logs", "comments", "named OAuth connection", "exact positional order"} {
		if !strings.Contains(content, want) {
			t.Fatalf("Deluge catalog must contain %q", want)
		}
	}
}

type zohoProjectsOperationRecord struct {
	Family            string
	OperationIdentity string
	OperationName     string
	Method            string
	Path              string
	Version           string
	HierarchyIDs      string
	Scopes            string
	PlanState         string
	LifecycleClass    string
	OfficialURL       string
	ObservedOn        string
	EvidenceState     string
}

func readZohoProjectsOperationCatalog(t *testing.T) []zohoProjectsOperationRecord {
	t.Helper()
	reader := csv.NewReader(strings.NewReader(readZohoProjectsAsset(t, "references/current-rest-operations.csv")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse Projects operation catalog: %v", err)
	}
	wantHeader := []string{"family", "operation_identity", "operation_name", "method", "path", "version", "hierarchy_ids", "scopes", "plan_state", "lifecycle_class", "official_url", "observed_on", "evidence_state"}
	if len(rows) == 0 || strings.Join(rows[0], "|") != strings.Join(wantHeader, "|") {
		t.Fatalf("Projects operation catalog header = %v; want %v", rows[0], wantHeader)
	}

	records := make([]zohoProjectsOperationRecord, 0, len(rows)-1)
	for rowIndex, row := range rows[1:] {
		if len(row) != len(wantHeader) {
			t.Fatalf("Projects operation catalog row %d has %d columns; want %d", rowIndex+2, len(row), len(wantHeader))
		}
		for columnIndex, value := range row {
			if strings.TrimSpace(value) == "" {
				t.Fatalf("Projects operation catalog row %d column %q is empty", rowIndex+2, wantHeader[columnIndex])
			}
		}
		records = append(records, zohoProjectsOperationRecord{
			Family: row[0], OperationIdentity: row[1], OperationName: row[2], Method: row[3], Path: row[4], Version: row[5],
			HierarchyIDs: row[6], Scopes: row[7], PlanState: row[8], LifecycleClass: row[9], OfficialURL: row[10],
			ObservedOn: row[11], EvidenceState: row[12],
		})
	}
	return records
}

func TestZohoProjectsEmbeddedSkill_RecordsCurrentRESTOperationCatalog(t *testing.T) {
	records := readZohoProjectsOperationCatalog(t)
	identities := make(map[string][]zohoProjectsOperationRecord)
	families := make(map[string]bool)
	versions := make(map[string]int)
	for _, record := range records {
		identities[record.OperationIdentity] = append(identities[record.OperationIdentity], record)
		families[record.Family] = true
		versions[record.Version]++
		if !strings.HasPrefix(record.Path, "/api/"+record.Version+"/") {
			t.Errorf("operation %q path %q does not match version %q", record.OperationIdentity, record.Path, record.Version)
		}
		if record.ObservedOn != "2026-09-01" {
			t.Errorf("operation %q observation date = %q; want 2026-09-01", record.OperationIdentity, record.ObservedOn)
		}
		if record.OfficialURL != "https://projects.zoho.com/api-docs#"+record.OperationIdentity {
			t.Errorf("operation %q official URL = %q", record.OperationIdentity, record.OfficialURL)
		}
		if record.EvidenceState != "verified" && record.EvidenceState != "contradictory" {
			t.Errorf("operation %q evidence state = %q; want verified or contradictory", record.OperationIdentity, record.EvidenceState)
		}
	}
	if len(records) != 489 || len(identities) != 478 || len(families) != 29 || versions["v3"] != 467 || versions["v3.1"] != 22 {
		t.Fatalf("operation catalog = %d sections, %d identities, %d families, %d v3, %d v3.1; want 489, 478, 29, 467, 22", len(records), len(identities), len(families), versions["v3"], versions["v3.1"])
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesOfficialFamilyLabels(t *testing.T) {
	wantFamilyByIdentityPrefix := map[string]struct {
		family string
		count  int
	}{
		"module_meta-":           {family: "Module Meta", count: 32},
		"clients_and_customers-": {family: "Clients And Customers", count: 20},
	}
	gotCounts := make(map[string]int, len(wantFamilyByIdentityPrefix))
	for _, record := range readZohoProjectsOperationCatalog(t) {
		for prefix, want := range wantFamilyByIdentityPrefix {
			if !strings.HasPrefix(record.OperationIdentity, prefix) {
				continue
			}
			gotCounts[prefix]++
			if record.Family != want.family {
				t.Errorf("operation %q family = %q; want exact official label %q", record.OperationIdentity, record.Family, want.family)
			}
		}
	}
	for prefix, want := range wantFamilyByIdentityPrefix {
		if gotCounts[prefix] != want.count {
			t.Errorf("operation family prefix %q has %d rows; want %d", prefix, gotCounts[prefix], want.count)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_UsesOfficialFamilyLabelsAcrossInstalledTree(t *testing.T) {
	destination := t.TempDir()
	if err := InstallSelected(jarvis.SkillsFS, destination, []string{"zoho-projects"}); err != nil {
		t.Fatalf("InstallSelected Projects tree: %v", err)
	}

	installedFS := os.DirFS(destination)
	legacyLabels := []string{"Module Metadata", "Clients and Customers"}
	if err := fs.WalkDir(installedFS, "zoho-projects", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(installedFS, name)
		if err != nil {
			return err
		}
		for _, legacyLabel := range legacyLabels {
			for range strings.Count(string(content), legacyLabel) {
				t.Errorf("installed Projects asset %q contains legacy family label %q", name, legacyLabel)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("scan installed Projects tree: %v", err)
	}

	wantLabelsByAsset := map[string]map[string]string{
		"zoho-projects/references/current-rest-api.md": {
			"Module Meta":           "| Module Meta |",
			"Clients And Customers": "| Clients And Customers |",
		},
		"zoho-projects/references/identifiers-and-metadata.md": {
			"Module Meta": "from Module Meta or",
		},
	}
	for name, wantLabels := range wantLabelsByAsset {
		content, err := fs.ReadFile(installedFS, name)
		if err != nil {
			t.Fatalf("read installed Projects asset %q: %v", name, err)
		}
		for wantLabel, wantText := range wantLabels {
			if !strings.Contains(string(content), wantText) {
				t.Errorf("installed Projects asset %q must contain official family label %q", name, wantLabel)
			}
		}
	}
}

func TestZohoProjectsEmbeddedSkill_ClassifiesLifecycleOperationsSemantically(t *testing.T) {
	allowedClasses := map[string]bool{
		"not-lifecycle": true, "trash": true, "restore": true,
		"activate": true, "deactivate": true, "activate-or-deactivate": true,
		"clone": true, "move": true, "reorder": true,
		"follow": true, "unfollow": true, "follow-or-unfollow": true,
		"link": true, "unlink": true, "associate": true, "disassociate": true, "associate-or-disassociate": true,
		"import": true, "export": true, "timer": true, "pin": true, "unpin": true,
		"default-selection": true, "favourite-selection": true,
		"status-history": true, "blueprint-or-status-transition": true,
		"custom-function-execution": true, "asynchronous-bulk-job": true,
	}
	wantMoveIdentities := map[string]bool{
		"phases-move_phase": true,
		"issues-move_issue": true,
		"tasks-move_task":   true,
	}
	wantClasses := map[string]string{
		"module_meta-modules-update_status":                                   "activate-or-deactivate",
		"users-users_invitation_templates-remove_default_invitation_template": "default-selection",
		"phases-phase_followers-remove_followers":                             "unfollow",
		"issues-issue_followers-remove_followers":                             "unfollow",
		"tasks-task_custom_view-remove_view_from_favorites_portal":            "favourite-selection",
		"tasks-task_custom_view-remove_view_from_favorites_project":           "favourite-selection",
		"custom_module_records-record_followers-add_or_remove_followers":      "follow-or-unfollow",
		"custom_module_records-record_followers-remove_follower":              "unfollow",
		"teams-project_teams-add_team_project":                                "associate",
		"teams-project_teams-remove_project_team":                             "disassociate",
		"attachments-attach_files":                                            "associate",
		"setup-business_hours-associate_or_disassociate_user":                 "associate-or-disassociate",
	}

	seen := make(map[string]bool, len(wantClasses))
	seenMoveIdentities := make(map[string]bool, len(wantMoveIdentities))
	for _, record := range readZohoProjectsOperationCatalog(t) {
		if !allowedClasses[record.LifecycleClass] {
			t.Errorf("operation %q lifecycle class %q is outside the approved taxonomy", record.OperationIdentity, record.LifecycleClass)
		}
		if record.LifecycleClass == "move" {
			seenMoveIdentities[record.OperationIdentity] = true
			if !wantMoveIdentities[record.OperationIdentity] {
				t.Errorf("operation %q is classified as move without move semantics", record.OperationIdentity)
			}
		}

		semanticText := strings.ToLower(strings.Join([]string{record.OperationIdentity, record.OperationName, record.Path}, " "))
		switch {
		case strings.Contains(semanticText, "associate_or_disassociate") || strings.Contains(semanticText, "associate or disassociate"):
			if record.LifecycleClass != "associate-or-disassociate" {
				t.Errorf("operation %q has associate-or-disassociate semantics but lifecycle class %q", record.OperationIdentity, record.LifecycleClass)
			}
		case strings.Contains(semanticText, "disassociate") || strings.Contains(semanticText, "dissociate"):
			if record.LifecycleClass != "disassociate" {
				t.Errorf("operation %q has disassociate semantics but lifecycle class %q", record.OperationIdentity, record.LifecycleClass)
			}
		case strings.Contains(semanticText, "associate") || strings.Contains(semanticText, "associated"):
			if record.LifecycleClass != "associate" {
				t.Errorf("operation %q has associate semantics but lifecycle class %q", record.OperationIdentity, record.LifecycleClass)
			}
		}

		want, ok := wantClasses[record.OperationIdentity]
		if !ok {
			continue
		}
		seen[record.OperationIdentity] = true
		if record.LifecycleClass != want {
			t.Errorf("operation %q lifecycle class = %q; want %q", record.OperationIdentity, record.LifecycleClass, want)
		}
	}
	for identity := range wantClasses {
		if !seen[identity] {
			t.Errorf("lifecycle operation %q is missing from catalog", identity)
		}
	}
	for identity := range wantMoveIdentities {
		if !seenMoveIdentities[identity] {
			t.Errorf("move operation %q is missing its lifecycle classification", identity)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PinsCurrentRESTOperationCatalogSnapshot(t *testing.T) {
	const wantSHA256 = "c45ab12b7ba30bd8c063ea9d700f85f57505275fda5e50b6ed08f9750dabd8b1"
	lfContent := strings.ReplaceAll(readZohoProjectsAsset(t, "references/current-rest-operations.csv"), "\r\n", "\n")
	tests := []struct {
		name    string
		content string
	}{
		{name: "LF checkout", content: lfContent},
		{name: "CRLF checkout", content: strings.ReplaceAll(lfContent, "\n", "\r\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logicalContent := strings.ReplaceAll(tt.content, "\r\n", "\n")
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(logicalContent))); got != wantSHA256 {
				t.Fatalf("Projects operation catalog SHA-256 = %s; want %s", got, wantSHA256)
			}
		})
	}
}

func TestZohoProjectsEmbeddedSkill_PinsOfficialRESTIndexSourceSnapshot(t *testing.T) {
	content := readZohoProjectsAsset(t, "references/sources.md")
	for _, want := range []string{
		"https://projects.zoho.com/api-docs",
		"2026-09-01",
		"ad063863d5751caa6fa94bac27fc5b222fdbeedb0a172873d0c6964836710259",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Projects source snapshot must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesEveryContradictoryOperationIdentity(t *testing.T) {
	wantContradictions := map[string]bool{
		"users-get_user_details":                       true,
		"users-update_user":                            true,
		"users-project_users-get_project_user_details": true,
		"issues-get_status_transition":                 true,
		"issues-issue_linking-get_linked_issues":       true,
		"issues-issue_linking-link_issues":             true,
		"issues-issue_linking-unlink_issues":           true,
		"issues-issue_resolution-get_resolution":       true,
		"issues-issue_resolution-add_resolution":       true,
		"issues-issue_resolution-update_resolution":    true,
		"issues-issue_resolution-delete_resolution":    true,
	}
	contradictions := make(map[string][]zohoProjectsOperationRecord)
	for _, record := range readZohoProjectsOperationCatalog(t) {
		if record.EvidenceState == "contradictory" {
			contradictions[record.OperationIdentity] = append(contradictions[record.OperationIdentity], record)
		}
	}
	if len(contradictions) != len(wantContradictions) {
		t.Fatalf("contradictory identities = %d; want %d", len(contradictions), len(wantContradictions))
	}
	for identity := range wantContradictions {
		variants := contradictions[identity]
		if len(variants) != 2 {
			t.Fatalf("contradictory identity %q has %d variants; want 2", identity, len(variants))
		}
		first := variants[0].Method + "|" + variants[0].Path + "|" + variants[0].Version + "|" + variants[0].Scopes
		second := variants[1].Method + "|" + variants[1].Path + "|" + variants[1].Version + "|" + variants[1].Scopes
		if first == second {
			t.Fatalf("contradictory identity %q does not preserve distinct method/path/version/scope variants", identity)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_RecordsCurrentRESTFamilyManifest(t *testing.T) {
	content := readZohoProjectsAsset(t, "references/current-rest-api.md")
	rows := 0
	sections := 0
	v3Sections := 0
	v31Sections := 0
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Family ") || strings.HasPrefix(line, "|---") {
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) != 8 {
			continue
		}
		values := make([]int, 3)
		for i, column := range columns[2:5] {
			value, err := strconv.Atoi(strings.TrimSpace(column))
			if err != nil {
				t.Fatalf("parse REST family row %q: %v", line, err)
			}
			values[i] = value
		}
		rows++
		sections += values[0]
		v3Sections += values[1]
		v31Sections += values[2]
	}
	if rows != 29 || sections != 489 || v3Sections != 467 || v31Sections != 22 {
		t.Fatalf("REST family manifest = %d families, %d sections, %d v3, %d v3.1; want 29, 489, 467, 22", rows, sections, v3Sections, v31Sections)
	}
	for _, want := range []string{"478 unique operation identities", "11 duplicated operation identities", "method", "exact path", "hierarchy IDs", "scope", "plan state", "lifecycle class", "official fragment URL", "2026-09-01", "evidence state", "[operation-level catalog](current-rest-operations.csv)", "A family row never authorizes an endpoint"} {
		if !strings.Contains(content, want) {
			t.Fatalf("REST catalog contract must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_RequiresAssumptionsAndExpectedTestOutcomes(t *testing.T) {
	content := readZohoProjectsAsset(t, "SKILL.md")
	for _, want := range []string{
		"Assumptions:",
		"Expected test outcomes:",
		"state each inferred or runtime-dependent fact",
		"pair each generated test case with its expected result",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Projects output contract must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesIdentitySafetyAndRuntimeAuthority(t *testing.T) {
	content := readZohoProjectsAsset(t, "references/identifiers-and-metadata.md")
	for _, want := range []string{
		"portal ID", "portal name", "project ID", "project key", "project name", "ZPUID", "email",
		"field ID", "column_name", "display name", "Projects attachment ID", "WorkDrive upload resource ID",
		"runtime metadata", "exact operation",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("identifier contract must contain %q", want)
		}
	}
}

func TestZohoProjectsEmbeddedSkill_PreservesOperationalSafetyBoundaries(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{"references/current-rest-api.md", []string{"pagination", "Retry-After", "200 requests per endpoint in two minutes", "ten minutes", "independent"}},
		{"references/authentication-limits-and-plans.md", []string{"Never request or embed secrets", "100 Projects integration-task executions in two minutes", "30-minute restriction", "independent", "runtime validation", "Enterprise-only", "paid plans"}},
		{"references/lifecycle-and-uncertainty.md", []string{"trash", "restore", "activate", "deactivate", "clone", "move", "reorder", "follow", "unfollow", "link", "unlink", "associate", "disassociate", "import", "export", "timers", "pins", "default", "favourite", "blueprint transitions", "custom-function execution", "bulk jobs", "explicit confirmation", "calendar events", "not webhook", "unsupported"}},
		{"references/routing.md", []string{"metadata", "permissions", "documents", "attachments", "setup", "automation", "custom modules", "bulk", "standard Projects functionality", "recommend one and wait"}},
	}
	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoProjectsAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}
