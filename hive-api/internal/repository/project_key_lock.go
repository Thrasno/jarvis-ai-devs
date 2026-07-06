package repository

import (
	"context"
	"sort"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/projectkey"
)

type ProjectKeyLockRepository interface {
	LockCanonicalProjectKeys(ctx context.Context, canonicalKeys []string) error
}

func CanonicalProjectKeys(projects []string) []string {
	seen := make(map[string]struct{}, len(projects))
	keys := make([]string, 0, len(projects))
	for _, project := range projects {
		canonical := projectkey.Canonicalize(project)
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		keys = append(keys, canonical)
	}
	sort.Strings(keys)
	return keys
}
