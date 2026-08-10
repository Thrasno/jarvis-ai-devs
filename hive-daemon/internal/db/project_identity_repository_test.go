package db

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
)

func TestProjectRepositoryIngressAndLookupUseCanonicalIdentity(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	memory := &models.Memory{Project: " Foo.Bar ", Title: "memory", Content: "content"}
	_, err := d.SaveMemoryWithManualSession(memory)
	require.NoError(t, err)
	require.Equal(t, "foo-bar", memory.Project)

	memories, err := d.ListMemories("FOO_BAR", 10)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, "foo-bar", memories[0].Project)

	prompt, err := d.SavePrompt(ctx, "foo/bar", "prompt")
	require.NoError(t, err)
	require.Equal(t, "foo-bar", prompt.Project)
	prompts, err := d.ListRecentPrompts(ctx, "FOO.BAR", 10)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Equal(t, "foo-bar", prompts[0].Project)

	require.NoError(t, d.CreateSession("case-session", "FOO-BAR", "/repo", "dev", "test"))
	sessions, err := d.ListSessions("foo/bar", 10)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	project, err := d.SessionProject(ctx, "case-session")
	require.NoError(t, err)
	require.Equal(t, "foo-bar", project)

	known, err := d.KnownProjects(ctx)
	require.NoError(t, err)
	require.Len(t, known, 1)
	require.Equal(t, "Foo.Bar", known[0].Name)
	require.Equal(t, "/repo", known[0].Directory)
}

func TestGovernanceProjectsUseCanonicalKeysAndRegistryDisplay(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	_, err := d.SaveMemoryWithManualSession(&models.Memory{Project: " Foo/Archive ", Title: "archive", Content: "content"})
	require.NoError(t, err)
	_, err = d.RawDB().Exec(`UPDATE project_identities SET remote_spelling = 'remote/archive' WHERE project_key = 'foo-archive'`)
	require.NoError(t, err)

	detail, err := d.GetGovernanceProject(ctx, "FOO_ARCHIVE")
	require.NoError(t, err)
	require.Equal(t, "remote/archive", detail.Name)

	archived, err := d.ArchiveGovernanceProject(ctx, "foo.archive", "actor", "reason", time.Time{})
	require.NoError(t, err)
	require.True(t, archived)

	_, err = d.SaveMemoryWithManualSession(&models.Memory{Project: " Source/Project ", Title: "source", Content: "content"})
	require.NoError(t, err)
	merged, err := d.MergeGovernanceProject(ctx, "source_project", "Target/Project", "actor", "reason", time.Time{})
	require.NoError(t, err)
	require.True(t, merged)

	target, err := d.GetGovernanceProject(ctx, "target.project")
	require.NoError(t, err)
	require.Equal(t, "Target/Project", target.Name)
}
