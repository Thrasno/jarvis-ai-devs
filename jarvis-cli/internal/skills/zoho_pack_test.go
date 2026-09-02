package skills

import (
	"reflect"
	"testing"
)

func TestZohoPackMemberIDsAreCatalogDerivedSortedAndDefensive(t *testing.T) {
	pack := NewZohoPack([]Skill{
		{ID: "zoho-projects"},
		{ID: "go-testing"},
		{ID: "zoho-crm"},
		{ID: "zoho-expense"},
		{ID: "zoho-crm"},
		{ID: "phpunit-testing"},
	})

	want := []string{"zoho-crm", "zoho-expense", "zoho-projects"}
	if got := pack.MemberIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("MemberIDs() = %v, want %v", got, want)
	}

	members := pack.MemberIDs()
	members[0] = "changed"
	if got := pack.MemberIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("MemberIDs() after caller mutation = %v, want %v", got, want)
	}
}

func TestZohoPackSelectionAndExpansion(t *testing.T) {
	pack := NewZohoPack([]Skill{{ID: "zoho-crm"}, {ID: "zoho-deluge"}, {ID: "zoho-projects"}})

	tests := []struct {
		name          string
		recorded      []string
		selected      bool
		wantEligible  bool
		wantDesired   []string
		wantMissing   []string
		wantSelection []string
	}{
		{
			name:          "legacy anchor expands to all catalog members",
			recorded:      []string{"go-testing", "zoho-deluge", "missing-skill"},
			wantEligible:  true,
			wantDesired:   []string{"go-testing", "missing-skill", "zoho-crm", "zoho-deluge", "zoho-projects"},
			wantMissing:   []string{"zoho-crm", "zoho-projects"},
			wantSelection: []string{"go-testing", "missing-skill", "zoho-crm", "zoho-deluge", "zoho-projects"},
		},
		{
			name:          "orphan does not enroll pack",
			recorded:      []string{"go-testing", "zoho-crm"},
			wantEligible:  false,
			wantDesired:   []string{"go-testing", "zoho-crm"},
			wantMissing:   nil,
			wantSelection: []string{"go-testing", "zoho-crm", "zoho-deluge", "zoho-projects"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pack.Selected(tt.recorded); got != tt.wantEligible {
				t.Fatalf("Selected(%v) = %v, want %v", tt.recorded, got, tt.wantEligible)
			}
			desired, missing, eligible := pack.Expand(tt.recorded)
			if eligible != tt.wantEligible || !reflect.DeepEqual(desired, tt.wantDesired) || !reflect.DeepEqual(missing, tt.wantMissing) {
				t.Fatalf("Expand(%v) = (%v, %v, %v), want (%v, %v, %v)", tt.recorded, desired, missing, eligible, tt.wantDesired, tt.wantMissing, tt.wantEligible)
			}
			if got := pack.ApplySelection(tt.recorded, true); !reflect.DeepEqual(got, tt.wantSelection) {
				t.Fatalf("ApplySelection(%v, true) = %v, want %v", tt.recorded, got, tt.wantSelection)
			}
		})
	}
}

func TestZohoPackDeselectionRemovesAllZohoIDsAndIsIdempotent(t *testing.T) {
	pack := NewZohoPack([]Skill{{ID: "zoho-crm"}, {ID: "zoho-deluge"}})
	recorded := []string{"go-testing", "zoho-deluge", "zoho-orphan", "go-testing"}

	if got, want := pack.ApplySelection(recorded, false), []string{"go-testing", "go-testing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ApplySelection(%v, false) = %v, want %v", recorded, got, want)
	}
	selected := pack.ApplySelection(recorded, true)
	if got := pack.ApplySelection(selected, true); !reflect.DeepEqual(got, selected) {
		t.Fatalf("reapplying selected pack = %v, want %v", got, selected)
	}
}
