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
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "zero defaults to 100", input: 0, want: 100},
		{name: "negative defaults to 100", input: -5, want: 100},
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
