package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	errSddWorkspaceUnresolved = errors.New("SDD workspace authority cannot be resolved")
	errSddWorkspaceAmbiguous  = errors.New("SDD workspace authority is ambiguous")
	errSddWorkspaceUnsafe     = errors.New("SDD workspace authority is unsafe")
	errSddNotGitWorktree      = errors.New("target is not a Git worktree")
)

var (
	sddTempDir = os.TempDir
)

// resolveSddWorkspaceAuthority returns the only directory phase agents may edit.
// It is deliberately independent of the Hive project name: --project is an
// identity alias, never authority to edit a different workspace.
func resolveSddWorkspaceAuthority(workingDir string) (string, error) {
	return resolveSddWorkspaceAuthorityWithGitDiscovery(workingDir, gitWorktreeRoot)
}

func resolveSddWorkspaceAuthorityWithGitDiscovery(workingDir string, findGitRoot func(string) (string, error)) (string, error) {
	candidate, err := canonicalSddWorkspaceDirectory(workingDir)
	if err != nil {
		return "", err
	}

	gitRoot, err := findGitRoot(candidate)
	if errors.Is(err, errSddNotGitWorktree) {
		return candidate, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: discover Git worktree: %v", errSddWorkspaceUnresolved, err)
	}
	if strings.TrimSpace(gitRoot) == "" {
		return "", fmt.Errorf("%w: Git worktree root is empty", errSddWorkspaceUnresolved)
	}
	root, err := canonicalSddWorkspaceDirectory(gitRoot)
	if err != nil {
		return "", err
	}
	inside, err := isWithinSddWorkspace(root, candidate)
	if err != nil || !inside {
		return "", fmt.Errorf("%w: git worktree root is not an ancestor of the target directory", errSddWorkspaceAmbiguous)
	}
	return root, nil
}

func canonicalSddWorkspaceDirectory(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("%w: target directory is empty", errSddWorkspaceUnresolved)
	}

	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("%w: make target absolute: %v", errSddWorkspaceUnresolved, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("%w: stat target directory: %v", errSddWorkspaceUnresolved, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: target directory is a symlink", errSddWorkspaceUnsafe)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: target is not a directory", errSddWorkspaceUnsafe)
	}
	traversesSymlink, err := hasSddWorkspaceSymlinkComponent(absolute)
	if err != nil {
		return "", fmt.Errorf("%w: inspect target directory: %v", errSddWorkspaceUnresolved, err)
	}
	if traversesSymlink {
		return "", fmt.Errorf("%w: target directory traverses a symlink", errSddWorkspaceUnsafe)
	}

	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("%w: evaluate target directory: %v", errSddWorkspaceUnresolved, err)
	}
	canonical = filepath.Clean(canonical)
	if isSddWorkspaceBroadPath(canonical) {
		return "", fmt.Errorf("%w: target directory is overly broad", errSddWorkspaceUnsafe)
	}
	return canonical, nil
}

func hasSddWorkspaceSymlinkComponent(target string) (bool, error) {
	return hasSddWorkspaceSymlinkComponentWithLstat(target, os.Lstat)
}

func hasSddWorkspaceSymlinkComponentWithLstat(target string, lstat func(string) (os.FileInfo, error)) (bool, error) {
	volume := filepath.VolumeName(target)
	root := volume + string(filepath.Separator)
	remaining := strings.TrimPrefix(target, root)
	current := root

	for _, component := range strings.Split(remaining, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

func isSddWorkspaceBroadPath(path string) bool {
	if path == filepath.Dir(path) {
		return true
	}
	temporaryRoot, err := filepath.EvalSymlinks(sddTempDir())
	if err != nil {
		return true
	}
	if path == filepath.Clean(temporaryRoot) {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return true
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return true
	}
	return path == filepath.Clean(home)
}

func gitWorktreeRoot(workingDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = workingDir
	cmd.Env = gitDiscoveryEnv()
	out, err := cmd.Output()
	if err != nil {
		if isNotGitWorktreeError(err) {
			return "", errSddNotGitWorktree
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git returned an empty worktree root")
	}
	return root, nil
}

func gitDiscoveryEnv() []string {
	ignored := map[string]bool{
		"GIT_DIR":                 true,
		"GIT_WORK_TREE":           true,
		"GIT_COMMON_DIR":          true,
		"GIT_CEILING_DIRECTORIES": true,
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !ignored[name] {
			env = append(env, value)
		}
	}
	return append(env, "LC_ALL=C")
}

func isNotGitWorktreeError(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 {
		return false
	}
	stderr := strings.ToLower(string(exitErr.Stderr))
	return strings.Contains(stderr, "not a git repository") || strings.Contains(stderr, "not a git work tree")
}

func isWithinSddWorkspace(root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}
