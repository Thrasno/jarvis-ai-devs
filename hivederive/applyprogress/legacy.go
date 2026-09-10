package applyprogress

// LegacyProgress is the conservative, already-parsed view of a cumulative v1 artifact.
type LegacyProgress struct {
	Tasks     []Task
	Completed []string
}

type LegacyConversion struct {
	Tasks              []Task
	Completed          []string
	TaskManifestSHA256 string
}

// ConvertLegacy normalizes legacy tasks and refuses ambiguous completion markers.
func ConvertLegacy(progress LegacyProgress) (LegacyConversion, error) {
	tasks, manifest, err := TaskManifest(progress.Tasks)
	if err != nil {
		return LegacyConversion{}, invalid(CodeLegacyAmbiguous)
	}
	byPath, identities := map[string]string{}, map[string]bool{}
	for _, task := range tasks {
		if !validID(task.Path) || byPath[task.Path] != "" {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous)
		}
		byPath[task.Path], identities[task.ID] = task.ID, true
	}
	completed, used := make([]string, len(progress.Completed)), map[string]bool{}
	for i, marker := range progress.Completed {
		id := marker
		if pathID := byPath[marker]; pathID != "" {
			if identities[marker] && marker != pathID {
				return LegacyConversion{}, invalid(CodeLegacyAmbiguous)
			}
			id = pathID
		}
		if !identities[id] || used[id] {
			return LegacyConversion{}, invalid(CodeLegacyAmbiguous)
		}
		used[id], completed[i] = true, id
	}
	return LegacyConversion{Tasks: tasks, Completed: completed, TaskManifestSHA256: manifest}, nil
}
