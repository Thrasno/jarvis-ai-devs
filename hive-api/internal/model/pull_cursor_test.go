package model_test

import (
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestPullCursor_IsZero(t *testing.T) {
	tests := []struct {
		name   string
		cursor model.PullCursor
		want   bool
	}{
		{name: "zero value", cursor: model.PullCursor{}, want: true},
		{name: "only synced_at set", cursor: model.PullCursor{SyncedAt: time.Now()}, want: false},
		{name: "only sync_id set", cursor: model.PullCursor{SyncID: "abc"}, want: false},
		{name: "both set", cursor: model.PullCursor{SyncedAt: time.Now(), SyncID: "abc"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cursor.IsZero())
		})
	}
}

func TestClampPullLimit(t *testing.T) {
	// ClampPullLimit only normalizes an EXPLICIT opt-in (limit > 0). Absent/0/negative
	// means "client did not opt into pagination" and must stay unbounded (see
	// model.UnboundedPullLimit) — true backward compat with pre-2a daemons that
	// always did a single unbounded pull. This is NOT a regression: unbounded pull
	// is the current status quo for clients that never send pull_limit.
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "zero means unbounded (client did not opt in)", input: 0, want: model.UnboundedPullLimit},
		{name: "negative means unbounded (client did not opt in)", input: -5, want: model.UnboundedPullLimit},
		{name: "within range preserved", input: 42, want: 42},
		{name: "one is preserved (lower bound)", input: 1, want: 1},
		{name: "exactly max preserved", input: 100, want: 100},
		{name: "above max clamps to 100", input: 500, want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, model.ClampPullLimit(tt.input))
		})
	}
}
