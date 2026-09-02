package skills

import (
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

const zohoPeopleSkillRoot = "embed/skills/zoho-people"

var zohoPeopleAssetNames = []string{
	"SKILL.md",
	"references/routing.md",
	"references/deluge-tasks.md",
	"references/rest-v1-v2-catalog.md",
	"references/rest-v3-catalog.md",
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

func TestZohoPeopleEmbeddedSkill_RegistersAndInstallsItsCompleteTree(t *testing.T) {
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
	for _, name := range zohoPeopleAssetNames {
		if _, err := fs.Stat(os.DirFS(destination), path.Join("zoho-people", name)); err != nil {
			t.Fatalf("installed People asset %q: %v", name, err)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_UsesOnlyInstalledLocalReferences(t *testing.T) {
	content := readZohoPeopleAsset(t, "SKILL.md")
	for _, name := range zohoPeopleAssetNames[1:] {
		if !strings.Contains(content, "("+name+")") {
			t.Fatalf("SKILL.md must link installed reference %q", name)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_StatesActivationRoutingAndSafetyContracts(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{"SKILL.md", []string{"host or target", "zoho-deluge", "standard People functionality", "runtime metadata", "contradictory", "unavailable", "TBD", "explicit confirmation", "bulk or destructive", "minimize personal data", "placeholders", "logs"}},
		{"references/routing.md", []string{"host", "target", "execution context", "API family", "operation", "plan", "every involved application skill", "equally optimal", "wait for selection"}},
		{"references/uncertainty-and-errors.md", []string{"only missing facts", "fail closed", "safe clarification", "unsupported", "Do not invent"}},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoPeopleAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
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
			t.Fatalf("Deluge catalog must include %q", task)
		}
	}
	if got := strings.Count(content, "| `zoho.people."); got != len(allowlist) {
		t.Fatalf("Deluge catalog has %d task rows; want exactly %d", got, len(allowlist))
	}
	for _, want := range []string{
		"maximum 200 records", "positional order", "searchField", "field link name",
		"form and field label names", "People record ID", "recordid", "named OAuth connections",
		"not applicable in Creator", "mandatory in Cliq", "one external call",
		"specialized leave, attendance, time, cases, LMS, compensation, performance, survey, files, webhook, or HR-process operations",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Deluge contract must contain %q", want)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_CatalogsContainEveryActiveOperation(t *testing.T) {
	tests := []struct {
		asset        string
		apiFamily    string
		wantRows     int
		familyCounts map[string]int
	}{
		{
			asset:     "references/rest-v1-v2-catalog.md",
			apiFamily: "v1/v2",
			wantRows:  263,
			familyCounts: map[string]int{
				"Organization": 1, "Forms": 9, "Cases": 4, "Timesheet": 41, "Onboarding": 5,
				"Announcements": 10, "Leave Tracker": 20, "Attendance": 27, "Compensation": 23,
				"Record Count": 1, "LMS": 107, "Files": 10, "Views": 3,
				"Standalone Function": 1, "Module Online Tests": 1,
			},
		},
		{
			asset:     "references/rest-v3-catalog.md",
			apiFamily: "v3",
			wantRows:  152,
			familyCounts: map[string]int{
				"Leave Tracker": 39, "Attendance": 7, "Shifts": 5, "Timesheet": 14, "Variables": 10,
				"Files": 16, "Performance": 32, "Organization Structure": 4,
				"Employee Engagement Surveys": 23, "HR Process": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.apiFamily, func(t *testing.T) {
			rows := parsePeopleCatalogRows(t, readZohoPeopleAsset(t, tt.asset))
			if len(rows) != tt.wantRows {
				t.Fatalf("%s has %d operation rows; want %d", tt.asset, len(rows), tt.wantRows)
			}

			gotFamilyCounts := make(map[string]int)
			seen := make(map[string]string)
			for rowIndex, row := range rows {
				if row[1] != tt.apiFamily {
					t.Fatalf("%s row %d API family = %q; want %q", tt.asset, rowIndex+1, row[1], tt.apiFamily)
				}
				gotFamilyCounts[row[2]]++
				for column, value := range row {
					if strings.TrimSpace(value) == "" {
						t.Fatalf("%s row %d column %d is empty", tt.asset, rowIndex+1, column+1)
					}
				}
				if !strings.HasPrefix(row[10], "https://") || row[11] != "2026-09-01" {
					t.Fatalf("%s row %d must contain an official URL and snapshot date, got %q and %q", tt.asset, rowIndex+1, row[10], row[11])
				}
				switch row[12] {
				case "verified", "contradictory", "unavailable", "TBD":
				default:
					t.Fatalf("%s row %d has invalid evidence state %q", tt.asset, rowIndex+1, row[12])
				}
				key := strings.Join([]string{row[1], row[2], row[0], row[3], row[4]}, "|")
				if priorState, duplicate := seen[key]; duplicate && (priorState != "contradictory" || row[12] != "contradictory") {
					t.Fatalf("%s duplicate operation identity %q must preserve contradictory evidence", tt.asset, key)
				}
				seen[key] = row[12]
			}

			for family, want := range tt.familyCounts {
				if got := gotFamilyCounts[family]; got != want {
					t.Errorf("%s family %q has %d rows; want %d", tt.asset, family, got, want)
				}
			}
		})
	}
}

func parsePeopleCatalogRows(t *testing.T, content string) [][]string {
	t.Helper()
	var rows [][]string
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Operation ") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 13 {
			t.Fatalf("catalog row has %d columns; want 13: %q", len(cells), line)
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	return rows
}

func TestZohoPeopleEmbeddedSkill_FailsClosedOnUnresolvedCatalogEvidence(t *testing.T) {
	assets := []string{"references/rest-v1-v2-catalog.md", "references/rest-v3-catalog.md"}
	totalRows := 0
	for _, asset := range assets {
		rows := parsePeopleCatalogRows(t, readZohoPeopleAsset(t, asset))
		totalRows += len(rows)
		for rowIndex, row := range rows {
			unresolvedSignature := peopleCatalogFieldIsUnresolved(row[4])
			unresolvedScope := peopleCatalogFieldIsUnresolved(row[7])
			if row[12] == "verified" && (unresolvedSignature || unresolvedScope) {
				t.Errorf("%s row %d %q is verified with unresolved signature %q or scope %q", asset, rowIndex+1, row[0], row[4], row[7])
			}
		}
	}
	if totalRows != 415 {
		t.Fatalf("catalog audit covered %d operation rows; want all 415", totalRows)
	}
}

func peopleCatalogFieldIsUnresolved(value string) bool {
	lowerValue := strings.ToLower(value)
	return strings.Contains(lowerValue, "tbd") ||
		strings.Contains(lowerValue, "unresolved") ||
		strings.Contains(lowerValue, "validate exact operation permission")
}

func TestZohoPeopleEmbeddedSkill_SanitizesCanonicalSignatures(t *testing.T) {
	for _, asset := range []string{"references/rest-v1-v2-catalog.md", "references/rest-v3-catalog.md"} {
		rows := parsePeopleCatalogRows(t, readZohoPeopleAsset(t, asset))
		for rowIndex, row := range rows {
			if row[12] != "verified" {
				continue
			}

			if problems := canonicalPeopleSignatureProblems(row[4]); len(problems) > 0 {
				t.Errorf("%s row %d %q has malformed canonical signature %q: %s", asset, rowIndex+1, row[0], row[4], strings.Join(problems, "; "))
			}
		}
	}
}

func TestCanonicalPeopleSignatureValidation(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		wantValid bool
	}{
		{name: "canonical path and selectors", signature: "https://people.zoho.com/people/api/attendance?empId={empId}&emailId={emailId}", wantValid: true},
		{name: "unbalanced braces", signature: "https://people.zoho.com/people/api/forms/{formLinkName"},
		{name: "nested braces", signature: "https://people.zoho.com/people/api/forms/{form{LinkName}}"},
		{name: "quote", signature: "https://people.zoho.com/people/api/files/upload\"?fileId={fileId}"},
		{name: "regex fragment and group", signature: `https://people.zoho.com/api/v3/function/(\w+)/execute`},
		{name: "optional route group", signature: "https://people.zoho.com/api/v1/courses/{courseId}/(batches/{batchId}/)learners/{learnerId}/course-progress"},
		{name: "whitespace", signature: "https://people.zoho.com/people/api/forms/{form LinkName}"},
		{name: "invalid host", signature: "https://people.zoho.example/people/api/forms"},
		{name: "header contamination", signature: "https://people.zoho.com/people/api/forms Authorization: Zoho-oauthtoken token"},
		{name: "malformed selector key", signature: "https://people.zoho.com/people/api/attendance?emailId{email}={emailId{email}}"},
		{name: "mismatched selector template", signature: "https://people.zoho.com/people/api/attendance?emailId={employeeId}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := canonicalPeopleSignatureProblems(tt.signature)
			if tt.wantValid && len(problems) > 0 {
				t.Fatalf("canonicalPeopleSignatureProblems(%q) = %v; want valid", tt.signature, problems)
			}
			if !tt.wantValid && len(problems) == 0 {
				t.Fatalf("canonicalPeopleSignatureProblems(%q) accepted malformed signature", tt.signature)
			}
		})
	}
}

func canonicalPeopleSignatureProblems(signature string) []string {
	var problems []string
	lowerSignature := strings.ToLower(signature)

	if strings.IndexFunc(signature, unicode.IsSpace) >= 0 {
		problems = append(problems, "contains whitespace")
	}
	if strings.ContainsAny(signature, `"'`+"`") {
		problems = append(problems, "contains a quote")
	}
	for _, contamination := range []string{
		"copied", "authorization:", "zoho-oauthtoken", "authtoken", "content-type:", "member'}",
		"zoho.comhttps://", "/people//api",
	} {
		if strings.Contains(lowerSignature, contamination) {
			problems = append(problems, "contains "+contamination)
		}
	}
	if strings.HasSuffix(lowerSignature, "/disabl") {
		problems = append(problems, "contains a truncated disable path")
	}
	if strings.Count(lowerSignature, "https://") != 1 {
		problems = append(problems, "must contain exactly one HTTPS URI")
	}

	parsed, err := url.Parse(signature)
	if err != nil {
		problems = append(problems, "cannot be parsed as a URI")
	} else {
		validHosts := map[string]bool{
			"people.zoho.com":    true,
			"people.zoho.com.au": true,
			"people.zoho.eu":     true,
			"people.zoho.in":     true,
			"people.zoho.com.cn": true,
			"people.zoho.jp":     true,
		}
		if parsed.Scheme != "https" || !validHosts[parsed.Hostname()] || parsed.Port() != "" || parsed.User != nil {
			problems = append(problems, "has an invalid People host")
		}
		if parsed.Fragment != "" {
			problems = append(problems, "contains a fragment")
		}
		if parsed.RawQuery != "" {
			selectorNamePattern := regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
			for _, selector := range strings.Split(parsed.RawQuery, "&") {
				parts := strings.Split(selector, "=")
				if len(parts) != 2 || !selectorNamePattern.MatchString(parts[0]) || parts[1] != "{"+parts[0]+"}" {
					problems = append(problems, "contains a malformed selector template")
					break
				}
			}
		}
	}

	depth := 0
	placeholderStart := -1
	placeholderNamePattern := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	for index, char := range signature {
		switch char {
		case '{':
			if depth != 0 {
				problems = append(problems, "contains nested braces")
			}
			depth++
			if depth == 1 {
				placeholderStart = index + 1
			}
		case '}':
			if depth == 0 {
				problems = append(problems, "contains an unmatched closing brace")
				continue
			}
			depth--
			if depth == 0 && !placeholderNamePattern.MatchString(signature[placeholderStart:index]) {
				problems = append(problems, "contains a malformed placeholder")
			}
		}
	}
	if depth != 0 {
		problems = append(problems, "contains an unmatched opening brace")
	}

	if strings.ContainsAny(signature, "()") {
		problems = append(problems, "contains parentheses or a regex/optional group")
	}
	if hasAdjacentDuplicatePeopleSelector(signature) {
		problems = append(problems, "repeats a path selector")
	}

	return problems
}

func hasAdjacentDuplicatePeopleSelector(signature string) bool {
	selectorPattern := regexp.MustCompile(`\{[^}]+\}`)
	selectors := selectorPattern.FindAllStringIndex(signature, -1)
	for i := 1; i < len(selectors); i++ {
		previous, current := selectors[i-1], selectors[i]
		if previous[1] == current[0] && signature[previous[0]:previous[1]] == signature[current[0]:current[1]] {
			return true
		}
	}
	return false
}

func TestZohoPeopleEmbeddedSkill_UsesOAuthOnlyCatalogMetadata(t *testing.T) {
	for _, asset := range []string{"references/rest-v1-v2-catalog.md", "references/rest-v3-catalog.md"} {
		rows := parsePeopleCatalogRows(t, readZohoPeopleAsset(t, asset))
		for rowIndex, row := range rows {
			for column := 4; column <= 7; column++ {
				if strings.Contains(strings.ToLower(row[column]), "authtoken") {
					t.Errorf("%s row %d %q contains legacy authtoken metadata in column %d", asset, rowIndex+1, row[0], column+1)
				}
			}
		}
	}

	authContract := readZohoPeopleAsset(t, "references/authentication-limits-and-plans.md")
	for _, want := range []string{"OAuth 2.0 only", "Do not generate legacy authtoken authentication"} {
		if !strings.Contains(authContract, want) {
			t.Errorf("authentication contract must contain %q", want)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_PreservesExactLifecycleDirection(t *testing.T) {
	directions := []string{"pause", "resume", "enable", "disable", "publish", "unpublish", "enroll", "unenroll"}
	wordPatterns := make(map[string]*regexp.Regexp, len(directions))
	for _, direction := range directions {
		wordPatterns[direction] = regexp.MustCompile(`\b` + direction + `\b`)
	}

	matched := make(map[string]int, len(directions))
	for _, asset := range []string{"references/rest-v1-v2-catalog.md", "references/rest-v3-catalog.md"} {
		rows := parsePeopleCatalogRows(t, readZohoPeopleAsset(t, asset))
		for rowIndex, row := range rows {
			operation := strings.ToLower(row[0])
			lifecycle := strings.ToLower(row[9])
			for _, direction := range directions {
				if !wordPatterns[direction].MatchString(operation) {
					continue
				}
				matched[direction]++
				if !wordPatterns[direction].MatchString(lifecycle) {
					t.Errorf("%s row %d %q lifecycle %q loses %q direction", asset, rowIndex+1, row[0], row[9], direction)
				}
			}
		}
	}
	for _, direction := range directions {
		if matched[direction] == 0 {
			t.Errorf("catalogs contain no lifecycle operation for %q", direction)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_PreservesEntityAndIdentifierBoundaries(t *testing.T) {
	content := readZohoPeopleAsset(t, "references/entity-identifiers.md")
	for _, want := range []string{
		"form label name", "form link name", "field label name", "field link name", "display name",
		"record ID", "employee ID", "email", "user ID", "erecno", "Zoho_ID", "pkId", "recordid",
		"People Time Tracker", "not Zoho Projects IDs", "General People Files", "LMS Files",
		"Specialized domains must not route through generic form CRUD",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("entity contract must contain %q", want)
		}
	}
}

func TestZohoPeopleEmbeddedSkill_UsesEndpointSpecificEmployeeIdentifiers(t *testing.T) {
	tests := []struct {
		name              string
		asset             string
		operation         string
		method            string
		signature         string
		requiredSelectors string
	}{
		{
			name:              "v3 add leave accepts exactly the documented employee alternatives",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Leave API / Add Leave Request API",
			method:            "POST",
			signature:         "https://people.zoho.com/people/api/v3/leave-tracker/leaves",
			requiredSelectors: "employee_zoho_id, employee_email_id, employee_id, Note: Any one parameter among employee_zoho_id, employee_email_id and employee_id is mandatory., approver_id",
		},
		{
			name:              "v3 edit leave requires the Zoho employee identifier",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Leave API / Edit Leave Request API",
			method:            "PUT",
			signature:         "https://people.zoho.com/people/api/v3/leave-tracker/leaves/{leave_record_id}",
			requiredSelectors: "employee_zoho_id*, approver_id, leave_record_id",
		},
		{
			name:              "v3 attendance entries supports its four documented employee selectors",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Get Entries API",
			method:            "GET",
			signature:         "https://people.zoho.com/people/api/v3/attendance/entries",
			requiredSelectors: "employee_email_id, employee_biometric_mapper_id, employee_id, employee_zoho_id, group_entries_by_employee",
		},
		{
			name:              "v3 shift schedule does not substitute the Zoho employee identifier",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Shift Schedule API / Get Schedule API",
			method:            "GET",
			signature:         "https://people.zoho.com/people/api/v3/shift/schedules",
			requiredSelectors: "employee_email_id, employee_biometric_mapper_id, employee_id",
		},
		{
			name:              "v3 shift mapping lookup uses the Zoho employee identifier",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Shift Mapping API / Get Mapping API",
			method:            "GET",
			signature:         "https://people.zoho.com/people/api/v3/shift/mapping",
			requiredSelectors: "employee_zoho_id",
		},
		{
			name:              "v3 shift assignment uses the employee identifier",
			asset:             "references/rest-v3-catalog.md",
			operation:         "Shift Mapping API / Map Shift API",
			method:            "POST",
			signature:         "https://people.zoho.com/people/api/v3/shift/mapping",
			requiredSelectors: "employee_id, shift_name, shift_id",
		},
		{
			name:              "v1 regularization uses erecno",
			asset:             "references/rest-v1-v2-catalog.md",
			operation:         "Regularization API / Add Regularization API",
			method:            "POST",
			signature:         "https://people.zoho.com/people/api/attendance/addRegularization",
			requiredSelectors: "erecno",
		},
		{
			name:              "v1 employee salary lookup uses erecno",
			asset:             "references/rest-v1-v2-catalog.md",
			operation:         "Fetch Single Employee Salary API",
			method:            "GET",
			signature:         "https://people.zoho.com/api/compensation/v1/salary/{erecno}",
			requiredSelectors: "erecno",
		},
		{
			name:              "form record lookup keeps record ID separate from employee identity",
			asset:             "references/rest-v1-v2-catalog.md",
			operation:         "Fetch Single Record API",
			method:            "GET",
			signature:         "https://people.zoho.com/api/forms/employee/getDataByID?recordId={recordId}",
			requiredSelectors: "formLinkName, recordId",
		},
	}

	catalogs := make(map[string][][]string)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, ok := catalogs[tt.asset]
			if !ok {
				rows = parsePeopleCatalogRows(t, readZohoPeopleAsset(t, tt.asset))
				catalogs[tt.asset] = rows
			}

			var matches [][]string
			for _, row := range rows {
				if row[0] == tt.operation && row[3] == tt.method && row[4] == tt.signature {
					matches = append(matches, row)
				}
			}
			if len(matches) != 1 {
				t.Fatalf("%s has %d rows for %s %s %s; want exactly one", tt.asset, len(matches), tt.method, tt.operation, tt.signature)
			}
			if got := matches[0][5]; got != tt.requiredSelectors {
				t.Fatalf("%s employee identifier contract = %q; want %q", tt.operation, got, tt.requiredSelectors)
			}
		})
	}
}

func TestZohoPeopleEmbeddedSkill_RequiresSafeAuthenticationLimitsPlansAndLifecycle(t *testing.T) {
	tests := []struct {
		asset string
		want  []string
	}{
		{
			asset: "references/authentication-limits-and-plans.md",
			want: []string{
				"OAuth 2.0", "named connections", "Never request or embed", "api_domain",
				"US", "AU", "EU", "IN", "CN", "JP", "one hour", "one-time", "until revoked",
				"Essential HR", "5,000", "Professional", "10,000", "Premium", "15,000", "Enterprise", "25,000",
				"endpoint thresholds", "lock periods", "TBD", "runtime validation",
			},
		},
		{
			asset: "references/lifecycle-and-webhooks.md",
			want: []string{
				"publish", "approve", "cancel", "pause", "resume", "enroll", "enable", "disable", "mark", "reminder",
				"product configuration", "not verified REST CRUD", "management endpoint", "payload", "retries", "signing", "TBD",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			content := readZohoPeopleAsset(t, tt.asset)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Fatalf("%s must contain %q", tt.asset, want)
				}
			}
		})
	}
}

func TestZohoPeopleEmbeddedSkill_KeepsVersionAndEvidenceBoundariesExplicit(t *testing.T) {
	for _, asset := range []string{"references/rest-v1-v2-catalog.md", "references/rest-v3-catalog.md"} {
		content := readZohoPeopleAsset(t, asset)
		for _, want := range []string{"242-operation alternate tree", "must not be merged", "exact official operation page", "Never rewrite", "Runtime metadata remains authoritative"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s must contain %q", asset, want)
			}
		}
	}
}
