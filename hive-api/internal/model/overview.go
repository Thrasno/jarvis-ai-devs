package model

// OverviewStatsResponse is the response for GET /admin/overview/stats.
type OverviewStatsResponse struct {
	DaemonHealth        OverviewDaemonHealth `json:"daemon_health"`
	Conflicts           OverviewConflicts    `json:"conflicts"`
	SyncHealthByProject []ProjectSyncHealth  `json:"sync_health_by_project"`
	LiveActivity        OverviewLiveActivity `json:"live_activity"`
	MostActiveProjects  []ProjectCount       `json:"most_active_projects"`
}

// OverviewDaemonHealth holds daemon health counts.
type OverviewDaemonHealth struct {
	Healthy int `json:"healthy"`
	Total   int `json:"total"`
}

// OverviewConflicts holds open conflict count.
type OverviewConflicts struct {
	Open int `json:"open"`
}

// ProjectSyncHealth holds per-project sync health status.
type ProjectSyncHealth struct {
	Project          string `json:"project"`
	Status           string `json:"status"` // healthy | degraded | unknown
	Region           string `json:"region"`
	ContributorCount int    `json:"contributor_count"`
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
