package httpapi

import (
	"reflect"
	"strings"
	"testing"
	"time"

	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthSummaryToResponse_BackoffUntil verifies that healthSummaryToResponse
// trusts the domain's BackoffUntil directly without re-checking time.Now().
//
// The domain (aggregate() in internal/sync/health.go) already guarantees that
// HealthSummary.BackoffUntil is zero for past values. The DTO must therefore
// treat any non-zero BackoffUntil as authoritative and set the pointer — it must
// NOT gate the field behind a second time.Now() check.
func TestHealthSummaryToResponse_BackoffUntil(t *testing.T) {
	// Use a fixed past time. The domain would never produce a non-zero past
	// BackoffUntil in production (aggregate() zeroes it), but if it does the
	// DTO must not silently drop it — that is the domain's responsibility.
	// Here we test the "trust the domain" contract: a non-zero BackoffUntil
	// MUST be forwarded, regardless of whether it is future or past.
	pastTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	summary := hivesync.HealthSummary{
		BackoffUntil: pastTime,
	}

	resp := healthSummaryToResponse(summary)

	require.NotNil(t, resp.BackoffUntil, "BackoffUntil must be non-nil when HealthSummary.BackoffUntil is non-zero")
	assert.Equal(t, pastTime.UTC(), *resp.BackoffUntil, "BackoffUntil must match the domain value")
}

// TestHealthSummaryToResponse_BackoffUntilZeroIsNil verifies that a zero
// BackoffUntil in the domain produces a nil pointer in the DTO.
func TestHealthSummaryToResponse_BackoffUntilZeroIsNil(t *testing.T) {
	summary := hivesync.HealthSummary{
		BackoffUntil: time.Time{},
	}

	resp := healthSummaryToResponse(summary)

	assert.Nil(t, resp.BackoffUntil, "zero BackoffUntil must map to nil")
}

// TestHealthSummaryToResponse_BackoffUntilFutureIsSet verifies that a future
// BackoffUntil is always forwarded as non-nil (no regression).
func TestHealthSummaryToResponse_BackoffUntilFutureIsSet(t *testing.T) {
	futureTime := time.Now().UTC().Add(10 * time.Minute)

	summary := hivesync.HealthSummary{
		BackoffUntil: futureTime,
	}

	resp := healthSummaryToResponse(summary)

	require.NotNil(t, resp.BackoffUntil, "future BackoffUntil must be non-nil")
	assert.Equal(t, futureTime.UTC(), *resp.BackoffUntil)
}

// TestHealthSummaryToResponse_LastDrainFieldsMapThrough pins PR 3 task 3.3:
// LastDrainState/LastDrainRemaining/LastDrainReason are ADDITIVE fields on
// both the domain HealthSummary and the wire DTO — mapping them through must
// not disturb any existing field.
func TestHealthSummaryToResponse_LastDrainFieldsMapThrough(t *testing.T) {
	summary := hivesync.HealthSummary{
		Reachable:           true,
		AuthOK:              true,
		AutoSync:            true,
		LastError:           "",
		ConsecutiveFailures: 0,
		UnsyncedMemories:    4,
		UnsyncedPrompts:     2,
		UnsyncedSessions:    1,
		LastDrainState:      "expected_pending",
		LastDrainReason:     "no-progress",
		LastDrainRemaining:  7,
	}

	resp := healthSummaryToResponse(summary)

	assert.Equal(t, "expected_pending", resp.LastDrainState)
	assert.Equal(t, "no-progress", resp.LastDrainReason)
	assert.Equal(t, 7, resp.LastDrainRemaining)

	// Existing fields must remain unaffected by the new mapping.
	assert.True(t, resp.Reachable)
	assert.True(t, resp.AuthOK)
	assert.True(t, resp.AutoSync)
	assert.Equal(t, "", resp.LastError)
	assert.Equal(t, 0, resp.ConsecutiveFailures)
	assert.Equal(t, 4, resp.UnsyncedMemories)
	assert.Equal(t, 2, resp.UnsyncedPrompts)
	assert.Equal(t, 1, resp.UnsyncedSessions)
}

// TestHealthSummaryResponse_ExistingFieldsUnchanged locks the frozen DTO
// field set (name + json tag) so a future change cannot silently rename,
// remove, or reorder an existing HealthSummaryResponse field — only additive
// new fields are allowed (PR 3 task 3.3 contract).
func TestHealthSummaryResponse_ExistingFieldsUnchanged(t *testing.T) {
	typ := reflect.TypeOf(HealthSummaryResponse{})

	wantJSONTags := map[string]string{
		"Reachable":           "reachable",
		"AuthOK":              "auth_ok",
		"AutoSync":            "auto_sync",
		"LastSuccessAt":       "last_success_at",
		"LastFailureAt":       "last_failure_at",
		"BackoffUntil":        "backoff_until",
		"LastError":           "last_error",
		"ConsecutiveFailures": "consecutive_failures",
		"UnsyncedMemories":    "unsynced_memories",
		"UnsyncedPrompts":     "unsynced_prompts",
		"UnsyncedSessions":    "unsynced_sessions",
	}

	for name, wantTag := range wantJSONTags {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("HealthSummaryResponse must still declare field %q", name)
		}
		gotTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if gotTag != wantTag {
			t.Fatalf("field %q json tag = %q, want %q", name, gotTag, wantTag)
		}
	}
}
