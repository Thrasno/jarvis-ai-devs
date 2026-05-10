package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuditMetadata_AllowlistByAction(t *testing.T) {
	tests := []struct {
		name   string
		action AuditAction
		input  map[string]any
		want   AuditMetadata
	}{
		{
			name:   "user level change stores only governance fields",
			action: AuditActionUserLevelChange,
			input: map[string]any{
				"target_username": "ada",
				"target_user_id":  "11111111-1111-1111-1111-111111111111",
				"old_level":       "member",
				"new_level":       "admin",
				"actor_username":  "root",
				"password":        "secret",
				"email":           "ada@example.com",
			},
			want: AuditMetadata{
				"target_username": "ada",
				"target_user_id":  "11111111-1111-1111-1111-111111111111",
				"old_level":       "member",
				"new_level":       "admin",
				"actor_username":  "root",
			},
		},
		{
			name:   "sync push stores only count summary fields",
			action: AuditActionSyncPush,
			input: map[string]any{
				"pushed_count":    3,
				"conflict_count":  1,
				"prompt_count":    2,
				"raw_payload":     "must-not-persist",
				"access_token":    "secret",
				"target_username": "wrong-action",
			},
			want: AuditMetadata{
				"pushed_count":   3,
				"conflict_count": 1,
				"prompt_count":   2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeAuditMetadata(tt.action, tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuditFilter_NormalizePagination(t *testing.T) {
	tests := []struct {
		name string
		in   AuditFilter
		want AuditFilter
	}{
		{
			name: "default limit is applied",
			in:   AuditFilter{},
			want: AuditFilter{Limit: 20},
		},
		{
			name: "limit is capped and negative offset is reset",
			in:   AuditFilter{Limit: 500, Offset: -10},
			want: AuditFilter{Limit: 100},
		},
		{
			name: "valid explicit page survives",
			in:   AuditFilter{Limit: 50, Offset: 25},
			want: AuditFilter{Limit: 50, Offset: 25},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalize()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAuditListResponse_StableEmptyList(t *testing.T) {
	resp := NewAuditListResponse(nil, 0, AuditFilter{Limit: 0, Offset: 5})

	assert.NotNil(t, resp.AuditLogs)
	assert.Empty(t, resp.AuditLogs)
	assert.EqualValues(t, 0, resp.Total)
	assert.Equal(t, 20, resp.Limit)
	assert.Equal(t, 5, resp.Offset)
}

func TestAuditEntry_TimestampsAreSerializable(t *testing.T) {
	occurredAt := time.Date(2026, 5, 10, 9, 30, 0, 0, time.UTC)
	entry := AuditEntry{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		OccurredAt: occurredAt,
		Action:     AuditActionUserDeactivate,
		Outcome:    AuditOutcomeSuccess,
		Metadata:   AuditMetadata{"target_username": "ada"},
	}

	assert.Equal(t, occurredAt, entry.OccurredAt)
	assert.Equal(t, AuditMetadata{"target_username": "ada"}, entry.Metadata)
}
