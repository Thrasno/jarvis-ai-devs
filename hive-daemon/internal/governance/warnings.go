package governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

type Warning struct {
	ID              int64
	CreatedAt       time.Time
	Severity        string
	Source          string
	Message         string
	ResolutionState string
	ResolvedAt      *time.Time
}

type WarningInput struct {
	Severity string
	Source   string
	Message  string
}

type WarningFilter struct {
	ResolutionState string
}

type warningStore interface {
	SaveHiveWarning(db.HiveWarningInput) (db.HiveWarning, error)
	ListHiveWarnings(db.HiveWarningFilter) ([]db.HiveWarning, error)
}

type WarningsService struct {
	store warningStore
}

func NewWarningsService(store warningStore) *WarningsService {
	return &WarningsService{store: store}
}

func (s *WarningsService) Record(ctx context.Context, input WarningInput) (Warning, error) {
	if err := ctx.Err(); err != nil {
		return Warning{}, err
	}
	input.Severity = strings.TrimSpace(input.Severity)
	input.Source = strings.TrimSpace(input.Source)
	input.Message = strings.TrimSpace(input.Message)
	if input.Severity == "" {
		return Warning{}, fmt.Errorf("severity is required")
	}
	if input.Source == "" {
		return Warning{}, fmt.Errorf("source is required")
	}
	if input.Message == "" {
		return Warning{}, fmt.Errorf("message is required")
	}

	warning, err := s.store.SaveHiveWarning(db.HiveWarningInput{
		Severity: input.Severity,
		Source:   input.Source,
		Message:  input.Message,
	})
	if err != nil {
		return Warning{}, err
	}
	return warningFromDB(warning), nil
}

func (s *WarningsService) List(ctx context.Context, filter WarningFilter) ([]Warning, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	warnings, err := s.store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: strings.TrimSpace(filter.ResolutionState)})
	if err != nil {
		return nil, err
	}
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, warningFromDB(warning))
	}
	return result, nil
}

func warningFromDB(warning db.HiveWarning) Warning {
	return Warning{
		ID:              warning.ID,
		CreatedAt:       warning.CreatedAt,
		Severity:        warning.Severity,
		Source:          warning.Source,
		Message:         warning.Message,
		ResolutionState: warning.ResolutionState,
		ResolvedAt:      warning.ResolvedAt,
	}
}
