package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type BackupStore struct {
	homeDir string
}

func NewBackupStore(homeDir string) BackupStore {
	return BackupStore{homeDir: homeDir}
}

func (s BackupStore) CreateSnapshot(sourceOperation string, targets []BackupTarget) (BackupManifest, error) {
	expanded, err := s.expandTargets(targets)
	if err != nil {
		return BackupManifest{}, err
	}
	snapshotID := fmt.Sprintf("snap-%d", time.Now().UnixNano())
	manifest := BackupManifest{
		SnapshotID:            snapshotID,
		SourceOperation:       sourceOperation,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339),
		JarvisVersion:         sddruntime.DefaultContract().JarvisVersion,
		ContractVersion:       sddruntime.DefaultContract().ContractVersion,
		ProviderSchemaVersion: sddruntime.DefaultContract().ProviderSchemaVersion,
		ArchivePath:           filepath.Join(s.backupDir(), snapshotID+".tar.gz"),
		Entries:               make([]BackupEntry, 0, len(expanded)),
	}
	for _, target := range expanded {
		raw, err := os.ReadFile(target.Path)
		if err != nil {
			return BackupManifest{}, err
		}
		sum := sha256.Sum256(raw)
		manifest.Entries = append(manifest.Entries, BackupEntry{Path: target.Path, Checksum: hex.EncodeToString(sum[:])})
	}
	if err := s.writeArchive(manifest.ArchivePath, expanded); err != nil {
		return BackupManifest{}, err
	}
	if err := s.saveManifest(manifest); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

// expandTargets snapshots regular files only. Recursive skill removal needs this
// to preserve every file before deletion. A caller-supplied leaf may be a symlink
// and is read through, as every earlier version did with os.ReadFile; only walked
// trees refuse links, so a managed tree can never cause a backup to read outside
// its root.
func (s BackupStore) expandTargets(targets []BackupTarget) ([]BackupTarget, error) {
	seen := make(map[string]bool)
	expanded := make([]BackupTarget, 0, len(targets))
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			expanded = append(expanded, BackupTarget{Path: path})
		}
	}
	for _, target := range targets {
		info, err := os.Lstat(target.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("backup target does not exist: %s", target.Path)
			}
			return nil, fmt.Errorf("backup target %s: %w", target.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// A linked directory is refused: walking through it could leave the
			// managed root. A linked file is archived through the link, as before.
			if info, err = os.Stat(target.Path); err != nil {
				return nil, fmt.Errorf("backup target %s: %w", target.Path, err)
			}
			if info.IsDir() {
				return nil, fmt.Errorf("backup target %s is a symlink to a directory", target.Path)
			}
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("backup target %s is not a regular file", target.Path)
			}
			add(target.Path)
			continue
		}
		err = filepath.WalkDir(target.Path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("backup target %s contains a symlink", path)
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("backup target %s is not a regular file", path)
			}
			add(path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(expanded, func(i, j int) bool { return expanded[i].Path < expanded[j].Path })
	return expanded, nil
}

// CreateSnapshotOfTargets snapshots a target list the caller computed itself,
// rather than one derived from doctor observations. A replay command knows what
// it is about to write and must back up exactly that, which differs from what
// doctor happens to consider managed today.
//
// Such a list describes desired state, so it names paths that do not exist yet:
// a skill this version added, or a statusline the user deleted. There is nothing
// to preserve for a missing file, and refusing over one would block every
// mutation permanently, so absent paths are skipped. Every target is still
// confined to the allowed roots, so a list assembled outside this package can
// never widen what a backup reads, and a refusal writes nothing.
//
// Existence decides which confinement check answers, because the two questions
// need different tools and only one of them works on an absent path:
//
//	the target cannot be looked at   -> report that failure as itself
//	the target exists                -> canonicalizing confinement (symlink-aware)
//	the target is absent, inside      -> skip: nothing to preserve
//	the target is absent, outside     -> refuse: the list overstepped
//
// The existence check therefore comes first, and its own failure is returned
// before any root is consulted: a permission error on the target is a fact about
// this machine, and answering it with a canonicalization failure about an
// unrelated root, or with a path-scope denial, sends the user after the wrong
// problem.
//
// An absent target is confined lexically because canonicalizing a path whose
// parent directory is absent is itself an error, and that absence is precisely
// the ordinary case this method exists to tolerate. The weaker check is enough
// here and only here: nothing is read for a path that is not there, so a
// symlinked ancestor cannot smuggle bytes past it. The moment the path exists,
// the canonicalizing check is the one that runs.
func (s BackupStore) CreateSnapshotOfTargets(sourceOperation string, targets []BackupTarget) (BackupManifest, error) {
	present := make([]BackupTarget, 0, len(targets))
	for _, target := range targets {
		if _, err := os.Lstat(target.Path); err != nil {
			if !os.IsNotExist(err) {
				return BackupManifest{}, err
			}
			if !s.isLexicallyAllowedRoot(target.Path) {
				return BackupManifest{}, fmt.Errorf("backup target outside allowed roots: %s", target.Path)
			}
			continue
		}
		allowed, err := s.isAllowedRoot(target.Path)
		if err != nil {
			return BackupManifest{}, err
		}
		if !allowed {
			return BackupManifest{}, fmt.Errorf("backup target outside allowed roots: %s", target.Path)
		}
		present = append(present, target)
	}
	return s.CreateSnapshot(sourceOperation, present)
}

func (s BackupStore) ValidateManifest(manifest BackupManifest) error {
	if len(manifest.Entries) == 0 {
		return fmt.Errorf("manifest contains no entries")
	}
	for _, entry := range manifest.Entries {
		ok, err := s.isAllowedRoot(entry.Path)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("path outside allowed roots %q", entry.Path)
		}
	}
	return nil
}

func (s BackupStore) saveManifest(manifest BackupManifest) error {
	if err := os.MkdirAll(s.backupDir(), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.manifestPath(manifest.SnapshotID), raw, 0o644)
}

func (s BackupStore) ValidateSnapshot(manifest BackupManifest) error {
	f, err := os.Open(manifest.ArchivePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := gz.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	tr := tar.NewReader(gz)
	checksums := make(map[string]string, len(manifest.Entries))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		checksums[restoreArchivePath(hdr.Name)] = hex.EncodeToString(sum[:])
	}

	for _, entry := range manifest.Entries {
		entryKey := archiveChecksumKey(entry.Path)
		got, ok := checksums[entryKey]
		if !ok {
			for _, legacyKey := range legacyArchiveChecksumKeys(entry.Path) {
				got, ok = checksums[legacyKey]
				if ok {
					break
				}
			}
		}
		if !ok {
			return fmt.Errorf("missing archive entry for %q", entry.Path)
		}
		if got != entry.Checksum {
			return fmt.Errorf("checksum mismatch for %q", entry.Path)
		}
	}
	return nil
}

func (s BackupStore) loadManifest(snapshotID string) (BackupManifest, error) {
	raw, err := os.ReadFile(s.manifestPath(snapshotID))
	if err != nil {
		return BackupManifest{}, err
	}
	var manifest BackupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

func (s BackupStore) backupDir() string {
	return filepath.Join(s.homeDir, ".jarvis", "backups")
}

func (s BackupStore) manifestPath(snapshotID string) string {
	return filepath.Join(s.backupDir(), snapshotID+".manifest.json")
}

// allowedRoots is the one list both confinement checks read, so the boundary
// cannot mean one thing for a path that exists and another for one that does not.
func (s BackupStore) allowedRoots() []string {
	return []string{
		filepath.Join(s.homeDir, ".claude"),
		filepath.Join(s.homeDir, ".config", "opencode"),
		filepath.Join(s.homeDir, ".jarvis"),
	}
}

// AllowedRoots exports allowedRoots so another package that must decide,
// before ever calling this store, whether a path it holds (e.g. a resolved
// symlink target) would fall outside this store's own confinement can derive
// that answer directly from the store instead of keeping a second,
// hand-copied list that could silently drift out of sync with this one
// (issue #767 hardening R2-004).
func (s BackupStore) AllowedRoots() []string {
	return s.allowedRoots()
}

// CanonicalizePath exports canonicalizePath so a caller outside this package
// can canonicalize a path (or one of this store's own AllowedRoots) exactly
// as this store canonicalizes a snapshot target before its confinement
// check, so the two comparisons can never disagree over a symlinked
// ancestor. ErrPathAbsent marks the ordinary case of a path (or its parent)
// that is simply not on this machine.
func CanonicalizePath(path string) (string, error) {
	return canonicalizePath(path)
}

// ErrPathAbsent is the exported form of the sentinel canonicalizePath
// returns for a path, or its parent, that does not exist on this machine.
var ErrPathAbsent = errPathAbsent

// isLexicallyAllowedRoot answers confinement without touching the filesystem,
// for a path that is not there to be touched. Clean collapses every ".." the
// caller wrote, so a path that climbs out of a root cannot climb back in on
// paper. What it cannot see is a symlinked ancestor, which is why it is only
// ever asked about a path with nothing behind it to read: it is a check on the
// caller's list, not on the machine.
//
// A relative path matches no root and is refused, which is the right answer for
// a target list that is supposed to name absolute managed paths.
func (s BackupStore) isLexicallyAllowedRoot(path string) bool {
	cleaned := filepath.Clean(path)
	for _, root := range s.allowedRoots() {
		canonRoot := filepath.Clean(root)
		if cleaned == canonRoot || strings.HasPrefix(cleaned, canonRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (s BackupStore) isAllowedRoot(path string) (bool, error) {
	canonPath, err := canonicalizePath(path)
	if err != nil {
		return false, fmt.Errorf("canonicalize restore path %q: %w", path, err)
	}
	for _, root := range s.allowedRoots() {
		canonRoot, err := canonicalizePath(root)
		if errors.Is(err, errPathAbsent) {
			// A root absent from this machine matches nothing. Not every user has
			// ~/.config/opencode, and treating its absence as fatal would refuse
			// every legitimate ~/.claude path on such a machine. Skipping an absent
			// root can only narrow what is allowed, never widen it.
			continue
		}
		if err != nil {
			// Anything else is a real problem with this machine rather than an
			// absent root: a permission failure, a symlink loop, an I/O error.
			// Swallowing it would deny a legitimate path as "outside allowed
			// roots", and Restore turns that refusal into restore_unsafe_path
			// advising the user to remove manifest entries — telling them to
			// destroy their recovery point over a permission bit.
			return false, fmt.Errorf("canonicalize allowed root %q: %w", root, err)
		}
		if canonPath == canonRoot || strings.HasPrefix(canonPath, canonRoot+string(os.PathSeparator)) {
			return true, nil
		}
	}
	return false, nil
}

func (s BackupStore) writeArchive(archivePath string, targets []BackupTarget) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gz := gzip.NewWriter(f)
	defer func() {
		if cerr := gz.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	tw := tar.NewWriter(gz)
	defer func() {
		if cerr := tw.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	for _, target := range targets {
		raw, err := os.ReadFile(target.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("backup target does not exist: %s", target.Path)
			}
			return err
		}
		hdr := &tar.Header{Name: archivePathName(target.Path), Mode: 0o644, Size: int64(len(raw)), Typeflag: tar.TypeReg, ModTime: time.Now()}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(raw); err != nil {
			return err
		}
	}
	return nil
}

func archivePathName(path string) string {
	key := archiveChecksumKey(path)
	if strings.HasPrefix(key, "/") && !strings.HasPrefix(key, "//") {
		return strings.TrimPrefix(key, "/")
	}
	return key
}

func restoreArchivePath(path string) string {
	return archiveChecksumKey(path)
}

func archiveChecksumKey(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")

	root := "/"
	rest := normalized
	if strings.HasPrefix(normalized, "//") {
		root = "//"
		rest = strings.TrimLeft(normalized, "/")
	} else if len(normalized) >= 2 && isWindowsDrivePrefix(normalized[:2]) {
		root = normalized[:2]
		rest = strings.TrimPrefix(normalized[2:], "/")
	} else {
		rest = strings.TrimLeft(normalized, "/")
	}

	cleaned := cleanArchiveSegments(rest)
	if root == "//" {
		if cleaned == "" {
			return root
		}
		return root + cleaned
	}
	if isWindowsDrivePrefix(root) {
		if cleaned == "" {
			return root
		}
		return root + "/" + cleaned
	}
	if cleaned == "" {
		return root
	}
	return root + cleaned
}

func legacyArchiveChecksumKeys(path string) []string {
	if hasParentArchiveSegment(path) {
		return nil
	}

	key := archiveChecksumKey(path)
	if strings.HasPrefix(key, "//") {
		return []string{"/" + strings.TrimPrefix(key, "//")}
	}
	return nil
}

func cleanArchiveSegments(path string) string {
	parts := strings.Split(path, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, part)
		}
	}
	return strings.Join(stack, "/")
}

func hasParentArchiveSegment(path string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isWindowsDrivePrefix(prefix string) bool {
	if len(prefix) != 2 || prefix[1] != ':' {
		return false
	}
	c := prefix[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// errPathAbsent marks the one canonicalization failure that is ordinary rather
// than broken: the path, or its parent, is simply not on this machine. Every
// other failure means something is wrong here and must not be mistaken for it.
var errPathAbsent = errors.New("path is absent")

func canonicalizePath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: parent path does not exist: %s", errPathAbsent, parent)
		}
		return "", err
	}
	return filepath.Join(filepath.Clean(resolvedParent), filepath.Base(abs)), nil
}
