package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

const maxSyncAttemptErrorMessageRune = 500

var ErrSyncAttemptBatchTooLarge = errors.New("sync attempt batch exceeds maximum size")

type SyncAttemptService interface {
	Ingest(ctx context.Context, req model.SyncAttemptIngestRequest) (model.SyncAttemptIngestResponse, error)
	Summary(ctx context.Context, query model.SyncAttemptSummaryQuery) (model.SyncAttemptSummaryResponse, error)
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

type syncAttemptService struct {
	repo repository.SyncAttemptRepository
}

func NewSyncAttemptService(repo repository.SyncAttemptRepository) SyncAttemptService {
	return &syncAttemptService{repo: repo}
}

func (s *syncAttemptService) Ingest(ctx context.Context, req model.SyncAttemptIngestRequest) (model.SyncAttemptIngestResponse, error) {
	if len(req.Attempts) > model.MaxSyncAttemptBatchSize {
		return model.SyncAttemptIngestResponse{}, ErrSyncAttemptBatchTooLarge
	}

	resp := model.SyncAttemptIngestResponse{}
	valid := make([]model.SyncAttemptLog, 0, len(req.Attempts))
	for _, payload := range req.Attempts {
		log, rejection := validateAndNormalizeSyncAttempt(payload)
		if rejection != nil {
			resp.Rejected = append(resp.Rejected, *rejection)
			continue
		}
		valid = append(valid, log)
	}

	if len(valid) == 0 {
		return resp, nil
	}
	stored, err := s.repo.UpsertBatch(ctx, valid)
	if err != nil {
		return model.SyncAttemptIngestResponse{}, err
	}
	if _, err := s.DeleteExpired(ctx, time.Now().UTC()); err != nil {
		log.Printf("warn: delete expired sync attempt logs: %v", err)
	}
	resp.AcceptedIDs = append(resp.AcceptedIDs, stored.AcceptedIDs...)
	resp.DuplicateIDs = append(resp.DuplicateIDs, stored.DuplicateIDs...)
	return resp, nil
}

func (s *syncAttemptService) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	return s.repo.DeleteOlderThan(ctx, now.UTC().AddDate(0, 0, -90))
}

func (s *syncAttemptService) Summary(ctx context.Context, query model.SyncAttemptSummaryQuery) (model.SyncAttemptSummaryResponse, error) {
	now := time.Now().UTC()
	since := now.Add(-30 * 24 * time.Hour)
	if query.Window != "" {
		since = now.Add(-summaryWindowDuration(query.Window))
	}
	records, err := s.repo.ListForSummary(ctx, model.SyncAttemptSummaryFilter{
		Since:     since,
		Project:   strings.TrimSpace(query.Project),
		DevID:     strings.TrimSpace(query.DevID),
		Client:    strings.TrimSpace(query.Client),
		DaemonID:  strings.TrimSpace(query.DaemonID),
		Outcome:   strings.TrimSpace(query.Outcome),
		ErrorCode: strings.TrimSpace(query.ErrorCode),
	})
	if err != nil {
		return model.SyncAttemptSummaryResponse{}, err
	}
	return BuildSyncAttemptSummary(records, query, now), nil
}

func BuildSyncAttemptSummary(records []model.SyncAttemptSummaryRecord, query model.SyncAttemptSummaryQuery, now time.Time) model.SyncAttemptSummaryResponse {
	windows := []string{"24h", "7d", "30d"}
	if query.Window != "" {
		windows = []string{query.Window}
	}

	resp := model.SyncAttemptSummaryResponse{Windows: make([]model.SyncAttemptWindowSummary, 0, len(windows))}
	for _, window := range windows {
		cutoff := now.UTC().Add(-summaryWindowDuration(window))
		collector := newSummaryCollector(window)
		for _, record := range records {
			if record.StartedAt.Before(cutoff) || !matchesSummaryQuery(record, query) {
				continue
			}
			collector.add(record)
		}
		resp.Windows = append(resp.Windows, collector.summary())
	}
	return resp
}

func summaryWindowDuration(window string) time.Duration {
	switch window {
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

func matchesSummaryQuery(record model.SyncAttemptSummaryRecord, query model.SyncAttemptSummaryQuery) bool {
	return matchesOptional(query.Project, record.Project) &&
		matchesOptional(query.DevID, record.DevID) &&
		matchesOptional(query.Client, record.Client) &&
		matchesOptional(query.DaemonID, record.DaemonID) &&
		matchesOptional(query.Outcome, string(record.Outcome)) &&
		matchesOptional(query.ErrorCode, stringValue(record.ErrorCode))
}

func matchesOptional(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	return expected == "" || actual == expected
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type summaryCollector struct {
	window      string
	total       int
	successes   int
	failures    int
	lastSuccess *time.Time
	lastFailure *time.Time
	developers  map[string]int
	projects    map[string]int
	clients     map[string]int
	daemons     map[string]int
	outcomes    map[string]int
	errors      map[string]int
}

func newSummaryCollector(window string) *summaryCollector {
	return &summaryCollector{
		window:     window,
		developers: map[string]int{},
		projects:   map[string]int{},
		clients:    map[string]int{},
		daemons:    map[string]int{},
		outcomes:   map[string]int{},
		errors:     map[string]int{},
	}
}

func (c *summaryCollector) add(record model.SyncAttemptSummaryRecord) {
	c.total++
	increment(c.developers, record.DevID)
	increment(c.projects, record.Project)
	increment(c.clients, record.Client)
	increment(c.daemons, record.DaemonID)
	increment(c.outcomes, string(record.Outcome))
	switch record.Outcome {
	case model.SyncAttemptOutcomeSuccess:
		c.successes++
		c.lastSuccess = maxTime(c.lastSuccess, record.StartedAt)
	case model.SyncAttemptOutcomeFailure:
		c.failures++
		c.lastFailure = maxTime(c.lastFailure, record.StartedAt)
		increment(c.errors, stringValue(record.ErrorCode))
	}
}

func (c *summaryCollector) summary() model.SyncAttemptWindowSummary {
	failureRate := 0.0
	if c.total > 0 {
		failureRate = float64(c.failures) / float64(c.total)
	}
	return model.SyncAttemptWindowSummary{
		Window:        c.window,
		Total:         c.total,
		Successes:     c.successes,
		Failures:      c.failures,
		FailureRate:   failureRate,
		LastSuccessAt: c.lastSuccess,
		LastFailureAt: c.lastFailure,
		ByDeveloper:   dimensionCounts(c.developers),
		ByProject:     dimensionCounts(c.projects),
		ByClient:      dimensionCounts(c.clients),
		ByDaemon:      dimensionCounts(c.daemons),
		ByOutcome:     dimensionCounts(c.outcomes),
		ByErrorCode:   dimensionCounts(c.errors),
		TopErrors:     dimensionCounts(c.errors),
	}
}

func increment(counts map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	counts[key]++
}

func maxTime(current *time.Time, candidate time.Time) *time.Time {
	candidate = candidate.UTC()
	if current == nil || candidate.After(*current) {
		return &candidate
	}
	return current
}

func dimensionCounts(counts map[string]int) []model.SyncAttemptDimensionCount {
	result := make([]model.SyncAttemptDimensionCount, 0, len(counts))
	for key, count := range counts {
		result = append(result, model.SyncAttemptDimensionCount{Key: key, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Key < result[j].Key
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func validateAndNormalizeSyncAttempt(payload model.SyncAttemptPayload) (model.SyncAttemptLog, *model.SyncAttemptRejected) {
	reject := func(message string) (model.SyncAttemptLog, *model.SyncAttemptRejected) {
		return model.SyncAttemptLog{}, &model.SyncAttemptRejected{AttemptID: payload.AttemptID, Error: message}
	}
	if strings.TrimSpace(payload.DevID) == "" {
		return reject("dev_id is required")
	}
	if strings.TrimSpace(payload.AttemptID) == "" {
		return reject("attempt_id is required")
	}
	if strings.TrimSpace(payload.Project) == "" {
		return reject("project is required")
	}
	if payload.StartedAt.IsZero() {
		return reject("started_at is required")
	}
	if payload.Outcome != model.SyncAttemptOutcomeSuccess && payload.Outcome != model.SyncAttemptOutcomeFailure {
		return reject(fmt.Sprintf("unsupported outcome %q", payload.Outcome))
	}

	log := model.SyncAttemptLog{
		AttemptID:  strings.TrimSpace(payload.AttemptID),
		DevID:      strings.TrimSpace(payload.DevID),
		Project:    strings.TrimSpace(payload.Project),
		Client:     strings.TrimSpace(payload.Client),
		DaemonID:   strings.TrimSpace(payload.DaemonID),
		StartedAt:  payload.StartedAt.UTC(),
		EndedAt:    payload.EndedAt,
		Outcome:    payload.Outcome,
		HTTPStatus: payload.HTTPStatus,
		ErrorCode:  payload.ErrorCode,
		RequestID:  strings.TrimSpace(payload.RequestID),
		SyncCounts: payload.SyncCounts,
		Metadata:   payload.Metadata,
	}
	if log.EndedAt != nil {
		ended := log.EndedAt.UTC()
		log.EndedAt = &ended
	}
	if payload.ErrorMessage != nil {
		cleaned := SanitizeSyncAttemptError(payload.DevID, *payload.ErrorMessage)
		log.ErrorMessage = &cleaned
	}
	return log, nil
}

func SanitizeSyncAttemptError(devID, message string) string {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.ReplaceAll(message, "\r", "\n")
	lines := strings.Split(message, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || isSensitiveHeaderLine(lower) {
			continue
		}
		if strings.Contains(lower, "request body") || strings.Contains(lower, "response body") {
			continue
		}
		if strings.HasPrefix(lower, "at ") || strings.Contains(lower, ".go:") || strings.Contains(lower, "goroutine ") || strings.Contains(lower, "stack trace") {
			continue
		}
		kept = append(kept, trimmed)
	}

	cleaned := strings.Join(kept, " ")
	cleaned = emailPattern.ReplaceAllString(cleaned, "[redacted-email]")
	if strings.TrimSpace(devID) != "" {
		cleaned = strings.ReplaceAll(cleaned, devID, "[redacted-email]")
	}
	cleaned = secretAssignmentPattern.ReplaceAllString(cleaned, "$1$2 [redacted]")
	cleaned = bearerPattern.ReplaceAllString(cleaned, "Bearer [redacted]")
	cleaned = localPathPattern.ReplaceAllString(cleaned, "[redacted-path]")
	cleaned = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, cleaned)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return truncateRunes(cleaned, maxSyncAttemptErrorMessageRune)
}

func isSensitiveHeaderLine(lower string) bool {
	key, _, ok := strings.Cut(lower, ":")
	if !ok || strings.Contains(key, " ") {
		return false
	}
	key = strings.TrimSpace(key)
	return key == "authorization" || key == "proxy-authorization" || key == "cookie" || key == "set-cookie" ||
		key == "x-api-key" || key == "api-key" || key == "x-auth-token"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

var (
	emailPattern            = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	bearerPattern           = regexp.MustCompile(`(?i)Bearer\s+[^\s,;]+`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(token|api[_-]?key|password|secret)\s*([=:])\s*([^\s,;]+)`)
	localPathPattern        = regexp.MustCompile(`(?i)(/home/[^\s,;)]+|/Users/[^\s,;)]+|/tmp/[^\s,;)]+|[A-Z]:\\[^\s,;)]+)`)
)
