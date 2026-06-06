package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

type Project = db.GovernanceProject
type Memory = db.GovernanceMemory
type Health = db.SyncHealth

var (
	ErrProjectRequired = errors.New("project is required")
	ErrProjectNotFound = errors.New("project not found")
)

type MemoryFilter struct {
	Project        string
	IncludeDeleted bool
	Limit          int
}

type readStore interface {
	ListGovernanceProjects(context.Context) ([]db.GovernanceProject, error)
	GetGovernanceProject(context.Context, string) (db.GovernanceProject, error)
	ListGovernanceMemories(context.Context, db.GovernanceMemoryFilter) ([]db.GovernanceMemory, error)
	ListGovernanceSyncHealth(context.Context) ([]db.SyncHealth, error)
}

type Service struct {
	store readStore
}

func NewService(store readStore) *Service {
	return &Service{store: store}
}

func (s *Service) Projects(ctx context.Context) ([]Project, error) {
	return s.store.ListGovernanceProjects(ctx)
}

func (s *Service) Project(ctx context.Context, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectRequired
	}
	project, err := s.store.GetGovernanceProject(ctx, name)
	return project, mapProjectError(err)
}

func (s *Service) Memories(ctx context.Context, filter MemoryFilter) ([]Memory, error) {
	project := strings.TrimSpace(filter.Project)
	if project == "" {
		return nil, ErrProjectRequired
	}
	if _, err := s.store.GetGovernanceProject(ctx, project); err != nil {
		return nil, mapProjectError(err)
	}
	return s.store.ListGovernanceMemories(ctx, db.GovernanceMemoryFilter{
		Project:        project,
		IncludeDeleted: filter.IncludeDeleted,
		Limit:          filter.Limit,
	})
}

func (s *Service) Health(ctx context.Context) ([]Health, error) {
	return s.store.ListGovernanceSyncHealth(ctx)
}

func mapProjectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, db.ErrGovernanceProjectRequired) {
		return ErrProjectRequired
	}
	if errors.Is(err, db.ErrGovernanceProjectNotFound) {
		return fmt.Errorf("%w: %v", ErrProjectNotFound, err)
	}
	return err
}
