package hiveui

import "testing"

func TestVerticalViewportRange(t *testing.T) {
	tests := []struct {
		name      string
		viewport  verticalViewport
		total     int
		wantStart int
		wantEnd   int
		wantLabel string
		wantAbove string
		wantBelow string
	}{
		{
			name:      "unbounded before terminal size is known",
			viewport:  verticalViewport{offset: 7},
			total:     12,
			wantStart: 0,
			wantEnd:   12,
			wantLabel: "1-12 of 12",
		},
		{
			name:      "known exhausted terminal reports no visible rows",
			viewport:  verticalViewport{bounded: true},
			total:     10,
			wantStart: 0,
			wantEnd:   0,
			wantLabel: "0 of 10",
			wantBelow: "↓ 10 more",
		},
		{
			name:      "bounded range reports overflow on both sides",
			viewport:  verticalViewport{height: 3, offset: 2},
			total:     10,
			wantStart: 2,
			wantEnd:   5,
			wantLabel: "3-5 of 10",
			wantAbove: "↑ 2 more",
			wantBelow: "↓ 5 more",
		},
		{
			name:      "empty range is deterministic",
			viewport:  verticalViewport{height: 3},
			wantStart: 0,
			wantEnd:   0,
			wantLabel: "0 of 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.viewport.rangeFor(tt.total)
			if got.Start != tt.wantStart || got.End != tt.wantEnd {
				t.Fatalf("range = %#v, want start %d end %d", got, tt.wantStart, tt.wantEnd)
			}
			if got.Label() != tt.wantLabel {
				t.Fatalf("Label() = %q, want %q", got.Label(), tt.wantLabel)
			}
			if got.MoreAbove() != tt.wantAbove || got.MoreBelow() != tt.wantBelow {
				t.Fatalf("overflow = above %q below %q, want above %q below %q", got.MoreAbove(), got.MoreBelow(), tt.wantAbove, tt.wantBelow)
			}
		})
	}
}

func TestVerticalViewportClamp(t *testing.T) {
	tests := []struct {
		name   string
		before verticalViewport
		total  int
		want   verticalViewport
	}{
		{
			name:   "negative offset clamps to zero",
			before: verticalViewport{height: 3, offset: -1},
			total:  10,
			want:   verticalViewport{height: 3},
		},
		{
			name:   "offset clamps to the last complete page",
			before: verticalViewport{height: 3, offset: 9},
			total:  10,
			want:   verticalViewport{height: 3, offset: 7},
		},
		{
			name:   "empty content has zero offset",
			before: verticalViewport{height: 3, offset: 4},
			want:   verticalViewport{height: 3},
		},
		{
			name:   "unknown height preserves unbounded pre-resize rendering",
			before: verticalViewport{offset: 4},
			total:  10,
			want:   verticalViewport{},
		},
		{
			name:   "known exhausted terminal clears its offset",
			before: verticalViewport{bounded: true, offset: 4},
			total:  10,
			want:   verticalViewport{bounded: true},
		},
		{
			name:   "negative height is safe and unbounded",
			before: verticalViewport{height: -2, offset: 4},
			total:  10,
			want:   verticalViewport{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.before.clamp(tt.total)
			if got != tt.want {
				t.Fatalf("clamp(%d) = %#v, want %#v", tt.total, got, tt.want)
			}
		})
	}
}
