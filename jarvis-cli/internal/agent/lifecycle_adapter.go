package agent

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/lifecycle"
)

type lifecycleAgentAdapter struct {
	agent Agent
}

func NewLifecycleAdapter(a Agent) lifecycle.ProviderAdapter {
	return &lifecycleAgentAdapter{agent: a}
}

func (a *lifecycleAgentAdapter) Name() string {
	return a.agent.Name()
}

func (a *lifecycleAgentAdapter) Observe() (lifecycle.ObservedProviderState, error) {
	observed, err := a.agent.ObserveRuntime()
	if err != nil {
		return lifecycle.ObservedProviderState{}, err
	}
	return lifecycle.ObservedProviderState{
		Artifacts:       observed.Artifacts,
		NonOwnedChanges: observed.NonOwnedChanges,
		UnknownChanges:  observed.UnknownChanges,
	}, nil
}

func (a *lifecycleAgentAdapter) Apply(steps []lifecycle.DoctorStep) error {
	for _, step := range steps {
		target, isDir, err := a.resolveManagedTarget(step.AssetID)
		if err != nil {
			return err
		}
		if isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("apply ensure dir %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("apply ensure parent %s: %w", filepath.Dir(target), err)
		}
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("apply stat %s: %w", target, err)
		}
		if err := os.WriteFile(target, []byte{}, 0o644); err != nil {
			return fmt.Errorf("apply ensure file %s: %w", target, err)
		}
	}
	return nil
}

func (a *lifecycleAgentAdapter) BackupTargets(steps []lifecycle.DoctorStep) ([]lifecycle.BackupTarget, error) {
	targets := make([]lifecycle.BackupTarget, 0, len(steps))
	seen := map[string]struct{}{}
	for _, step := range steps {
		target, _, err := a.resolveManagedTarget(step.AssetID)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, lifecycle.BackupTarget{Path: target})
	}
	return targets, nil
}

func (a *lifecycleAgentAdapter) Restore(manifest lifecycle.BackupManifest) (written int, err error) {
	f, err := os.Open(manifest.ArchivePath)
	if err != nil {
		return 0, fmt.Errorf("open archive: %w", err)
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("open gzip reader: %w", err)
	}
	defer func() {
		if cerr := gz.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	tr := tar.NewReader(gz)
	written = 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read archive entry: %w", err)
		}
		dst := archiveEntryPath(hdr.Name)
		if !a.isAllowedRestorePath(dst) {
			return 0, fmt.Errorf("restore path outside allowed roots: %s", dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return 0, fmt.Errorf("create restore parent: %w", err)
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return 0, fmt.Errorf("read archive payload: %w", err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return 0, fmt.Errorf("write restored file: %w", err)
		}
		written++
	}

	return written, nil
}

func (a *lifecycleAgentAdapter) resolveManagedTarget(assetID string) (path string, isDir bool, err error) {
	plan, err := a.agent.RuntimePlan()
	if err != nil {
		return "", false, fmt.Errorf("resolve runtime plan: %w", err)
	}

	var rel string
	for _, artifact := range plan.Contract.ManagedArtifacts {
		if artifact.ID == assetID {
			rel = artifact.RelativePath
			if rel == "" {
				rel = filepath.Base(plan.Paths.Instructions)
			}
			break
		}
	}
	if rel == "" {
		return "", false, fmt.Errorf("unsupported managed asset %q", assetID)
	}

	abs := filepath.Join(a.agent.ConfigDir(), rel)
	if !strings.HasPrefix(abs, a.agent.ConfigDir()) {
		return "", false, fmt.Errorf("asset path escapes provider config dir: %s", abs)
	}
	return abs, strings.HasSuffix(rel, "/") || strings.HasSuffix(rel, string(filepath.Separator)), nil
}

func archiveEntryPath(name string) string {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) {
		return clean
	}
	return string(filepath.Separator) + clean
}

func (a *lifecycleAgentAdapter) isAllowedRestorePath(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range a.allowedRestoreRoots() {
		cleanRoot := filepath.Clean(root)
		if cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (a *lifecycleAgentAdapter) allowedRestoreRoots() []string {
	cfg := filepath.Clean(a.agent.ConfigDir())
	home := filepath.Dir(cfg)
	if filepath.Base(cfg) == "opencode" && filepath.Base(filepath.Dir(cfg)) == ".config" {
		home = filepath.Dir(filepath.Dir(cfg))
	}

	return []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".config", "opencode"),
		filepath.Join(home, ".jarvis"),
	}
}
