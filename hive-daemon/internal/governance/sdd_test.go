package governance_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceFetchSDDArtifactsReturnsFiniteVocabulary(t *testing.T) {
	store, service := newSDDService(t)
	artifacts := []string{"explore", "proposal", "spec", "design", "tasks", "apply-progress", "verify-report", "archive-report"}
	for _, artifact := range artifacts {
		saveSDDServiceMemory(t, store, "project", "sdd/change/"+artifact, artifact)
	}
	saveSDDServiceMemory(t, store, "project", "sdd/change/unknown", "unknown")

	got, err := service.FetchSDDArtifacts(context.Background(), "project", "change")
	require.NoError(t, err)
	require.Len(t, got, len(artifacts))
	for i, artifact := range got {
		assert.Equal(t, artifacts[i], artifact.Artifact)
		assert.Equal(t, artifacts[i], artifact.Content)
	}
}

func TestServiceSDDValidation(t *testing.T) {
	_, service := newSDDService(t)
	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{name: "project", run: func() error { _, err := service.FetchSDDArtifacts(context.Background(), " ", "change"); return err }, want: governance.ErrProjectRequired},
		{name: "change", run: func() error { _, err := service.FetchSDDArtifacts(context.Background(), "project", " "); return err }, want: governance.ErrSDDChangeRequired},
		{name: "change slash", run: func() error { _, err := service.FetchSDDArtifacts(context.Background(), "project", "a/b"); return err }, want: governance.ErrSDDChangeInvalid},
		{name: "limit", run: func() error {
			_, err := service.ListSDDChanges(context.Background(), governance.SDDChangePageRequest{Project: "project", Limit: 0})
			return err
		}, want: governance.ErrSDDLimitInvalid},
		{name: "cursor", run: func() error {
			_, err := service.ListSDDChanges(context.Background(), governance.SDDChangePageRequest{Project: "project", Limit: 2, Cursor: "not-a-cursor"})
			return err
		}, want: governance.ErrSDDCursorInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.ErrorIs(t, tt.run(), tt.want) })
	}
}

func TestServiceListSDDChangesPaginatesAfterProjection(t *testing.T) {
	store, service := newSDDService(t)
	for _, change := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		for revision := 0; revision < 4; revision++ {
			saveSDDServiceMemory(t, store, "project", "sdd/"+change+"/explore", fmt.Sprintf("%s-%d", change, revision))
		}
	}

	first, err := service.ListSDDChanges(context.Background(), governance.SDDChangePageRequest{Project: "project", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo"}, first.Changes)
	assert.NotEmpty(t, first.NextCursor)
	second, err := service.ListSDDChanges(context.Background(), governance.SDDChangePageRequest{Project: "project", Limit: 2, Cursor: first.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"charlie", "delta"}, second.Changes)
	third, err := service.ListSDDChanges(context.Background(), governance.SDDChangePageRequest{Project: "project", Limit: 2, Cursor: second.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"echo"}, third.Changes)
	assert.Empty(t, third.NextCursor)
}

func newSDDService(t *testing.T) (*db.DB, *governance.Service) {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store, governance.NewService(store)
}

func saveSDDServiceMemory(t *testing.T, store *db.DB, project, topic, content string) {
	t.Helper()
	_, err := store.SaveMemoryWithManualSession(&models.Memory{Project: project, TopicKey: &topic, Title: topic, Content: content})
	require.NoError(t, err)
}
