package httpapi

import (
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
