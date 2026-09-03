package agent

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

func TestZohoPackInstallation_RealAgentsPreserveEmbeddedSkillTrees(t *testing.T) {
	catalog, err := skills.ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("list embedded skill catalog: %v", err)
	}
	selected := skills.NewZohoPack(catalog).MemberIDs()
	if !slices.Contains(selected, "zoho-deluge") {
		t.Fatalf("catalog-derived Zoho pack must include legacy skill %q; members: %v", "zoho-deluge", selected)
	}
	if !slices.Contains(selected, "zoho-projects") {
		t.Fatalf("catalog-derived Zoho pack must include %q; members: %v", "zoho-projects", selected)
	}

	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills filesystem: %v", err)
	}

	agents := []struct {
		name  string
		build func(home string) interface {
			InstallSkills(fs.FS, []string) error
			skillsDir() string
		}
	}{
		{
			name: "Claude Code",
			build: func(home string) interface {
				InstallSkills(fs.FS, []string) error
				skillsDir() string
			} {
				return &ClaudeAgent{home: home}
			},
		},
		{
			name: "OpenCode",
			build: func(home string) interface {
				InstallSkills(fs.FS, []string) error
				skillsDir() string
			} {
				return &OpenCodeAgent{home: home}
			},
		},
	}

	agentTrees := make(map[string]map[string][]byte, len(agents))
	for _, tt := range agents {
		t.Run(tt.name, func(t *testing.T) {
			agent := tt.build(t.TempDir())
			if err := agent.InstallSkills(skillsFS, selected); err != nil {
				t.Fatalf("install catalog-derived Zoho pack: %v", err)
			}

			firstInstall := make(map[string][]byte)
			for _, skillID := range selected {
				sourceRoot := path.Join("embed/skills", skillID)
				source := zohoSkillFileInventory(t, jarvis.SkillsFS, sourceRoot)
				generated := zohoSkillFileInventory(t, os.DirFS(agent.skillsDir()), skillID)
				assertZohoSkillInventoryEqual(t, skillID+" source", source, tt.name+" generated", generated)
				assertZohoMarkdownReferencesResolve(t, jarvis.SkillsFS, sourceRoot, skillID+" source")
				assertZohoMarkdownReferencesResolve(t, os.DirFS(agent.skillsDir()), skillID, tt.name+" generated")
				for relativePath, content := range generated {
					firstInstall[path.Join(skillID, relativePath)] = content
				}
			}

			if _, ok := firstInstall["zoho-projects/references/current-rest-operations.csv"]; !ok {
				t.Fatal("catalog-derived Zoho pack installation must include zoho-projects/references/current-rest-operations.csv")
			}
			if _, ok := firstInstall["zoho-deluge/SKILL.md"]; !ok {
				t.Fatal("catalog-derived Zoho pack installation must include the zoho-deluge tree")
			}

			if err := agent.InstallSkills(skillsFS, selected); err != nil {
				t.Fatalf("install unchanged catalog-derived Zoho pack a second time: %v", err)
			}
			secondInstall := zohoPackFileInventory(t, os.DirFS(agent.skillsDir()), selected)
			assertZohoSkillInventoryEqual(t, tt.name+" first install", firstInstall, tt.name+" second install", secondInstall)
			agentTrees[tt.name] = secondInstall
		})
	}

	assertZohoSkillInventoryEqual(t, "Claude Code generated", agentTrees["Claude Code"], "OpenCode generated", agentTrees["OpenCode"])
}

func zohoPackFileInventory(t *testing.T, fsys fs.FS, skillIDs []string) map[string][]byte {
	t.Helper()
	inventory := make(map[string][]byte)
	for _, skillID := range skillIDs {
		for relativePath, content := range zohoSkillFileInventory(t, fsys, skillID) {
			inventory[path.Join(skillID, relativePath)] = content
		}
	}
	return inventory
}

func zohoSkillFileInventory(t *testing.T, fsys fs.FS, root string) map[string][]byte {
	t.Helper()
	inventory := make(map[string][]byte)
	err := fs.WalkDir(fsys, root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(filePath))
		if err != nil {
			return err
		}
		inventory[filepath.ToSlash(relativePath)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return inventory
}

func assertZohoSkillInventoryEqual(t *testing.T, wantName string, want map[string][]byte, gotName string, got map[string][]byte) {
	t.Helper()
	var differences []string
	for _, relativePath := range sortedZohoInventoryPaths(want) {
		content, ok := got[relativePath]
		if !ok {
			differences = append(differences, fmt.Sprintf("missing from %s: %s", gotName, relativePath))
			continue
		}
		if !bytes.Equal(want[relativePath], content) {
			differences = append(differences, fmt.Sprintf("content differs: %s", relativePath))
		}
	}
	for _, relativePath := range sortedZohoInventoryPaths(got) {
		if _, ok := want[relativePath]; !ok {
			differences = append(differences, fmt.Sprintf("unexpected in %s: %s", gotName, relativePath))
		}
	}
	if len(differences) > 0 {
		t.Fatalf("%s and %s file inventories differ:\n%s", wantName, gotName, strings.Join(differences, "\n"))
	}
}

func sortedZohoInventoryPaths(inventory map[string][]byte) []string {
	paths := make([]string, 0, len(inventory))
	for relativePath := range inventory {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)
	return paths
}

var zohoMarkdownLinkPatterns = []*regexp.Regexp{
	regexp.MustCompile(`!?\[[^]]*\]\(\s*(?:<([^>]+)>|([^\s)]+))[^)]*\)`),
	regexp.MustCompile(`(?m)^\s{0,3}\[[^]]+\]:\s*(?:<([^>]+)>|([^\s]+))`),
}

func assertZohoMarkdownReferencesResolve(t *testing.T, fsys fs.FS, root, treeName string) {
	t.Helper()
	err := fs.WalkDir(fsys, root, func(markdownPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Ext(markdownPath) != ".md" {
			return nil
		}
		content, err := fs.ReadFile(fsys, markdownPath)
		if err != nil {
			return err
		}
		for _, pattern := range zohoMarkdownLinkPatterns {
			for _, match := range pattern.FindAllSubmatch(content, -1) {
				target := string(match[1])
				if target == "" {
					target = string(match[2])
				}
				localTarget, ok := zohoLocalMarkdownTarget(target)
				if !ok {
					continue
				}
				resolved := path.Clean(path.Join(path.Dir(markdownPath), localTarget))
				if resolved != root && !strings.HasPrefix(resolved, root+"/") {
					return fmt.Errorf("%s reference %q escapes its skill tree", markdownPath, target)
				}
				if _, err := fs.Stat(fsys, resolved); err != nil {
					return fmt.Errorf("%s local reference %q resolves to %q: %w", markdownPath, target, resolved, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resolve local Markdown references in %s: %v", treeName, err)
	}
}

func zohoLocalMarkdownTarget(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "\\") {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		return "", false
	}
	if parsed.Path == "" {
		return "", false
	}
	return parsed.Path, true
}
