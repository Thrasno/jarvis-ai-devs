package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type sddWorkspaceTestFileInfo struct {
	mode os.FileMode
}

func (info sddWorkspaceTestFileInfo) Name() string       { return "" }
func (info sddWorkspaceTestFileInfo) Size() int64        { return 0 }
func (info sddWorkspaceTestFileInfo) Mode() os.FileMode  { return info.mode }
func (info sddWorkspaceTestFileInfo) ModTime() time.Time { return time.Time{} }
func (info sddWorkspaceTestFileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info sddWorkspaceTestFileInfo) Sys() any           { return nil }

func TestHasSddWorkspaceSymlinkComponentUsesLstatNotPathNormalization(t *testing.T) {
	target := filepath.Join(t.TempDir(), "RUNNER~1", "workspace")
	symlinkComponent := filepath.Join(filepath.Dir(filepath.Dir(target)), "RUNNER~1")

	tests := []struct {
		name             string
		symlinkComponent string
		want             bool
	}{
		{
			name: "ordinary lexical normalization is not a symlink",
		},
		{
			name:             "actual symlink component is rejected",
			symlinkComponent: symlinkComponent,
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hasSddWorkspaceSymlinkComponentWithLstat(target, func(path string) (os.FileInfo, error) {
				if path == tt.symlinkComponent {
					return sddWorkspaceTestFileInfo{mode: os.ModeSymlink | 0o777}, nil
				}
				return sddWorkspaceTestFileInfo{mode: os.ModeDir | 0o755}, nil
			})
			if err != nil {
				t.Fatalf("hasSddWorkspaceSymlinkComponentWithLstat: %v", err)
			}
			if got != tt.want {
				t.Fatalf("hasSddWorkspaceSymlinkComponentWithLstat = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestResolveSddWorkspaceAuthorityFailsClosedOnGitDiscoveryFailures(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}

	tests := []struct {
		name     string
		discover func(string) (string, error)
		want     string
		wantErr  error
	}{
		{
			name: "authoritative non-git worktree result permits target",
			discover: func(string) (string, error) {
				return "", errSddNotGitWorktree
			},
			want: workingDir,
		},
		{
			name: "timeout fails closed",
			discover: func(string) (string, error) {
				return "", context.DeadlineExceeded
			},
			wantErr: errSddWorkspaceUnresolved,
		},
		{
			name: "missing executable fails closed",
			discover: func(string) (string, error) {
				return "", exec.ErrNotFound
			},
			wantErr: errSddWorkspaceUnresolved,
		},
		{
			name: "permission failure fails closed",
			discover: func(string) (string, error) {
				return "", os.ErrPermission
			},
			wantErr: errSddWorkspaceUnresolved,
		},
		{
			name: "malformed git response fails closed",
			discover: func(string) (string, error) {
				return "", errors.New("malformed git response")
			},
			wantErr: errSddWorkspaceUnresolved,
		},
		{
			name: "empty git worktree root fails closed",
			discover: func(string) (string, error) {
				return "", nil
			},
			wantErr: errSddWorkspaceUnresolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSddWorkspaceAuthorityWithGitDiscovery(workingDir, tt.discover)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("resolveSddWorkspaceAuthorityWithGitDiscovery error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveSddWorkspaceAuthorityWithGitDiscovery = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsSddWorkspaceBroadPathRejectsCanonicalizedSymlinkedTempRoot(t *testing.T) {
	temporaryRoot := t.TempDir()
	alias := filepath.Join(t.TempDir(), "temporary-root-alias")
	if err := os.Symlink(temporaryRoot, alias); err != nil {
		t.Fatalf("create temporary root alias: %v", err)
	}

	previous := sddTempDir
	sddTempDir = func() string { return alias }
	t.Cleanup(func() { sddTempDir = previous })

	canonical, err := filepath.EvalSymlinks(temporaryRoot)
	if err != nil {
		t.Fatalf("canonicalize temporary root: %v", err)
	}
	if !isSddWorkspaceBroadPath(canonical) {
		t.Fatalf("isSddWorkspaceBroadPath(%q) = false, want true for canonicalized temporary root", canonical)
	}
}

func TestResolveSddWorkspaceAuthorityRejectsUnrelatedGitRoot(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	unrelatedRoot := filepath.Join(t.TempDir(), "unrelated")
	if err := os.Mkdir(unrelatedRoot, 0o755); err != nil {
		t.Fatalf("create unrelated root: %v", err)
	}

	_, err := resolveSddWorkspaceAuthorityWithGitDiscovery(workingDir, func(string) (string, error) {
		return unrelatedRoot, nil
	})
	if !errors.Is(err, errSddWorkspaceAmbiguous) {
		t.Fatalf("resolveSddWorkspaceAuthorityWithGitRoot error = %v, want %v", err, errSddWorkspaceAmbiguous)
	}
}

func TestGitWorktreeRootDoesNotUseAmbientRepository(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "non-git-project")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatalf("create non-git project: %v", err)
	}

	if root, err := gitWorktreeRoot(workingDir); !errors.Is(err, errSddNotGitWorktree) {
		t.Fatalf("gitWorktreeRoot(%q) = (%q, %v), want authoritative non-Git result", workingDir, root, err)
	}
}

func TestResolveSddWorkspaceAuthorityPrefersGitWorktreeRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git")
	}

	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if out, err := exec.Command("git", "init", repository).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	workingDir := filepath.Join(repository, "nested", "project")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("create nested target: %v", err)
	}

	got, err := resolveSddWorkspaceAuthority(workingDir)
	if err != nil {
		t.Fatalf("resolveSddWorkspaceAuthority: %v", err)
	}
	if got != repository {
		t.Fatalf("resolveSddWorkspaceAuthority(%q) = %q, want git root %q", workingDir, got, repository)
	}
}

func TestResolveSddWorkspaceAuthorityRejectsUnsafeFilesystemTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	project := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatalf("create project directory: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("create file target: %v", err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatalf("create symlink target: %v", err)
	}

	tests := []struct {
		name    string
		target  string
		want    string
		wantErr error
	}{
		{name: "canonical non-git project", target: project, want: project},
		{name: "missing target", target: missing, wantErr: errSddWorkspaceUnresolved},
		{name: "file target", target: file, wantErr: errSddWorkspaceUnsafe},
		{name: "filesystem root", target: string(filepath.Separator), wantErr: errSddWorkspaceUnsafe},
		{name: "home directory", target: home, wantErr: errSddWorkspaceUnsafe},
		{name: "temporary root", target: os.TempDir(), wantErr: errSddWorkspaceUnsafe},
		{name: "symlink alias", target: alias, wantErr: errSddWorkspaceUnsafe},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSddWorkspaceAuthority(tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("resolveSddWorkspaceAuthority(%q) error = %v, want %v", tt.target, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveSddWorkspaceAuthority(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
