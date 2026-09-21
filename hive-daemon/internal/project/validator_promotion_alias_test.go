package project_test

import (
	"context"
	"os/exec"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func TestValidateWriteProjectDoesNotRepromoteGitAliasSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := t.TempDir()
	require.NoError(t, d.AddAlias(ctx, "git-source", "active", "local", "previous promotion"))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "active")
	require.NoError(t, err)
	initValidatorGitRepo(t, workspace, "https://github.com/org/git-source.git")

	result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace})
	require.NoError(t, err)
	require.Equal(t, "active", result.Project)
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "active", bound)
	_, found, err = d.ResolveAlias(ctx, "active")
	require.NoError(t, err)
	require.False(t, found, "active must not be re-promoted to a retired Git alias source")
}

func TestValidateWriteProjectPromotesToActiveGitAliasTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	workspace := t.TempDir()
	require.NoError(t, d.AddAlias(ctx, "retired-git-target", "active-git-target", "local", "later promotion"))
	_, _, err = d.EnsureWorkspaceProjectBinding(ctx, workspace, "directory-source")
	require.NoError(t, err)
	initValidatorGitRepo(t, workspace, "https://github.com/org/retired-git-target.git")

	result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace})
	require.NoError(t, err)
	require.Equal(t, "active-git-target", result.Project)
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "active-git-target", bound)
	_, found, err = d.ResolveAlias(ctx, "directory-source")
	require.NoError(t, err)
	require.True(t, found)
}
