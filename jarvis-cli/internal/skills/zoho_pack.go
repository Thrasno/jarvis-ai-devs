package skills

import (
	"sort"
	"strings"
)

const ZohoLegacyAnchorID = "zoho-deluge"

// ZohoPack is the catalog-derived selection contract for embedded Zoho skills.
type ZohoPack struct {
	memberIDs []string
}

// NewZohoPack derives the current pack from the supplied catalog.
func NewZohoPack(catalog []Skill) ZohoPack {
	members := make(map[string]bool)
	for _, skill := range catalog {
		if isZohoSkillID(skill.ID) {
			members[skill.ID] = true
		}
	}

	memberIDs := make([]string, 0, len(members))
	for id := range members {
		memberIDs = append(memberIDs, id)
	}
	sort.Strings(memberIDs)
	return ZohoPack{memberIDs: memberIDs}
}

// MemberIDs returns a defensive copy of the catalog-derived members.
func (p ZohoPack) MemberIDs() []string {
	return append([]string(nil), p.memberIDs...)
}

// Selected reports whether recorded state establishes Zoho pack enrollment.
func (p ZohoPack) Selected(recorded []string) bool {
	for _, id := range recorded {
		if id == ZohoLegacyAnchorID {
			return true
		}
	}
	return false
}

// Expand returns the selection required to converge an enrolled pack, the current
// catalog members absent from recorded state, and whether the state is enrolled.
func (p ZohoPack) Expand(recorded []string) (desired, missing []string, eligible bool) {
	if !p.Selected(recorded) {
		return append([]string(nil), recorded...), nil, false
	}

	recordedIDs := make(map[string]bool, len(recorded))
	for _, id := range recorded {
		recordedIDs[id] = true
	}
	for _, id := range p.memberIDs {
		if !recordedIDs[id] {
			missing = append(missing, id)
		}
	}
	return p.ApplySelection(recorded, true), missing, true
}

// ApplySelection replaces the Zoho subset while preserving non-Zoho ownership.
func (p ZohoPack) ApplySelection(recorded []string, selected bool) []string {
	result := make([]string, 0, len(recorded)+len(p.memberIDs))
	zohoIDs := make(map[string]bool)
	for _, id := range recorded {
		if isZohoSkillID(id) {
			if selected {
				zohoIDs[id] = true
			}
			continue
		}
		result = append(result, id)
	}
	if !selected {
		return result
	}
	for _, id := range p.memberIDs {
		zohoIDs[id] = true
	}
	members := make([]string, 0, len(zohoIDs))
	for id := range zohoIDs {
		members = append(members, id)
	}
	sort.Strings(members)
	return append(result, members...)
}

func isZohoSkillID(id string) bool {
	return strings.HasPrefix(id, "zoho-")
}
