package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	snapshotID := fmt.Sprintf("snap-%d", time.Now().UnixNano())
	manifest := BackupManifest{
		SnapshotID:            snapshotID,
		SourceOperation:       sourceOperation,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339),
		JarvisVersion:         sddruntime.DefaultContract().JarvisVersion,
		ContractVersion:       sddruntime.DefaultContract().ContractVersion,
		ProviderSchemaVersion: sddruntime.DefaultContract().ProviderSchemaVersion,
		ArchivePath:           filepath.Join(s.backupDir(), snapshotID+".tar.gz"),
		Entries:               make([]BackupEntry, 0, len(targets)),
	}
	for _, target := range targets {
		raw, err := os.ReadFile(target.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return BackupManifest{}, fmt.Errorf("backup target does not exist: %s", target.Path)
			}
			return BackupManifest{}, err
		}
		sum := sha256.Sum256(raw)
		manifest.Entries = append(manifest.Entries, BackupEntry{Path: target.Path, Checksum: hex.EncodeToString(sum[:])})
	}
	if err := s.writeArchive(manifest.ArchivePath, targets); err != nil {
		return BackupManifest{}, err
	}
	if err := s.saveManifest(manifest); err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
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

func (s BackupStore) isAllowedRoot(path string) (bool, error) {
	canonPath, err := canonicalizePath(path)
	if err != nil {
		return false, fmt.Errorf("canonicalize restore path %q: %w", path, err)
	}
	allowed := []string{
		filepath.Join(s.homeDir, ".claude"),
		filepath.Join(s.homeDir, ".config", "opencode"),
		filepath.Join(s.homeDir, ".jarvis"),
	}
	for _, root := range allowed {
		canonRoot, err := canonicalizePath(root)
		if err != nil {
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
			return "", fmt.Errorf("parent path does not exist: %s", parent)
		}
		return "", err
	}
	return filepath.Join(filepath.Clean(resolvedParent), filepath.Base(abs)), nil
}
