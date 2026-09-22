package hiveui

import (
	"fmt"
	"strings"
)

// verticalViewport tracks the visible central-content window. bounded marks
// that a terminal sizing message was received, distinguishing unknown height
// from a known terminal with no central content rows.
type verticalViewport struct {
	height  int
	offset  int
	bounded bool
}

// verticalRange describes a safe half-open visible range and its overflow.
type verticalRange struct {
	Start int
	End   int
	Total int
}

func (v verticalViewport) clamp(total int) verticalViewport {
	if v.height < 0 {
		return verticalViewport{}
	}
	if v.height == 0 {
		if v.bounded {
			return verticalViewport{bounded: true}
		}
		return verticalViewport{}
	}
	if total <= 0 {
		return verticalViewport{height: v.height, bounded: v.bounded}
	}
	if v.offset < 0 {
		v.offset = 0
	}
	maxOffset := total - v.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
	return v
}

func (v verticalViewport) rangeFor(total int) verticalRange {
	if total <= 0 {
		return verticalRange{}
	}
	v = v.clamp(total)
	if v.height == 0 {
		if v.bounded {
			return verticalRange{Total: total}
		}
		return verticalRange{End: total, Total: total}
	}
	visible := v.height
	if remaining := total - v.offset; visible > remaining {
		visible = remaining
	}
	return verticalRange{Start: v.offset, End: v.offset + visible, Total: total}
}

// Label returns a deterministic, one-based human-readable range label.
func (r verticalRange) Label() string {
	if r.Total <= 0 {
		return "0 of 0"
	}
	if r.End <= r.Start {
		return fmt.Sprintf("0 of %d", r.Total)
	}
	return fmt.Sprintf("%d-%d of %d", r.Start+1, r.End, r.Total)
}

// MoreAbove returns the deterministic overflow indicator above this range.
func (r verticalRange) MoreAbove() string {
	if r.Start <= 0 {
		return ""
	}
	return fmt.Sprintf("↑ %d more", r.Start)
}

// Feedback joins the visible range and any overflow into one stable line.
func (r verticalRange) Feedback() string {
	parts := []string{r.Label()}
	if above := r.MoreAbove(); above != "" {
		parts = append(parts, above)
	}
	if below := r.MoreBelow(); below != "" {
		parts = append(parts, below)
	}
	return strings.Join(parts, " · ")
}

// MoreBelow returns the deterministic overflow indicator below this range.
func (r verticalRange) MoreBelow() string {
	remaining := r.Total - r.End
	if remaining <= 0 {
		return ""
	}
	return fmt.Sprintf("↓ %d more", remaining)
}
