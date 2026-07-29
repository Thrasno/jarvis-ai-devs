package model

import "time"

// OverviewCapability identifies the data projection authorized for an overview.
type OverviewCapability string

const (
	OverviewCapabilityMember OverviewCapability = "member"
	OverviewCapabilityAdmin  OverviewCapability = "admin"
)

// OverviewSummary contains the aggregates available to every overview capability.
type OverviewSummary struct {
	TotalMemories      int64                      `json:"total_memories"`
	ActiveProjects     int                        `json:"active_projects"`
	LiveActivity       MemberOverviewLiveActivity `json:"live_activity"`
	MostActiveProjects []ProjectCount             `json:"most_active_projects"`
}

// MemberOverviewLiveActivity intentionally exposes only an aggregate count.
type MemberOverviewLiveActivity struct {
	Count int `json:"count"`
}

// CapabilityOverviewResponse is the capability-aware overview projection.
type CapabilityOverviewResponse struct {
	Capability OverviewCapability       `json:"capability"`
	Summary    OverviewSummary          `json:"summary"`
	Operations *AdminOverviewOperations `json:"operations,omitempty"`
}

// AdminOverviewOperations contains fields intentionally excluded from Member projections.
type AdminOverviewOperations struct {
	DaemonHealth        OverviewDaemonHealth     `json:"daemon_health"`
	DegradedProjects    OverviewDegradedProjects `json:"degraded_projects"`
	KnowledgeGrowth     []OverviewChartPoint     `json:"knowledge_growth"`
	SyncHealthByProject []ProjectSyncHealth      `json:"sync_health_by_project"`
	NewestSyncID        string                   `json:"newest_sync_id"`
}

// OverviewStatsResponse is the response for GET /admin/overview/stats.
type OverviewStatsResponse struct {
	DaemonHealth        OverviewDaemonHealth     `json:"daemon_health"`
	DegradedProjects    OverviewDegradedProjects `json:"degraded_projects"`
	SyncHealthByProject []ProjectSyncHealth      `json:"sync_health_by_project"`
	LiveActivity        OverviewLiveActivity     `json:"live_activity"`
	MostActiveProjects  []ProjectCount           `json:"most_active_projects"`
}

// OverviewDaemonHealth holds daemon health counts.
type OverviewDaemonHealth struct {
	Healthy int `json:"healthy"`
	Total   int `json:"total"`
}

// OverviewDegradedProjects holds the canonical degraded-project KPI.
type OverviewDegradedProjects struct {
	Degraded int `json:"degraded"`
	Total    int `json:"total"`
}

// ProjectSyncHealth holds per-project sync health status.
type ProjectSyncHealth struct {
	Project          string     `json:"project"`
	Status           string     `json:"status"` // healthy | degraded | unknown
	Region           string     `json:"region"`
	ContributorCount int        `json:"contributor_count"`
	LastActivityAt   *time.Time `json:"last_activity_at,omitempty"`
}

// OverviewLiveActivity holds recent sync activity metrics.
type OverviewLiveActivity struct {
	Count        int    `json:"count"`
	NewestSyncID string `json:"newest_sync_id"`
}

// OverviewChartPoint is a labeled data point for chart data.
type OverviewChartPoint struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// OverviewGrowthResponse is the response for GET /admin/overview/growth.
type OverviewGrowthResponse struct {
	KnowledgeGrowth []OverviewChartPoint `json:"knowledge_growth"`
}

// MonthCount holds a month label and cumulative value for growth queries.
type MonthCount struct {
	Label string
	Value int
}
