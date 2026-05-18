package sddruntime

import (
	"fmt"
	"strings"
)

type RuntimeManifestState struct {
	Present            bool
	Corrupted          bool
	ContractVersion    string
	ManagedArtifactIDs []string
}

type ObservedArtifact struct {
	Exists       bool
	MarkersValid bool
	SHA256       string
}

type ObservedRuntime struct {
	Manifest                 RuntimeManifestState
	RegistryPath             string
	PromptSourceIDs          []string
	StoreMode                string
	StoreReadFrom            []string
	StoreWriteTo             []string
	ArtifactTopics           []string
	GeneralMemoryTopics      []string
	ModelAssignments         map[string]string
	ResolvedModelAssignments map[string]string
	Artifacts                map[string]ObservedArtifact
	NonOwnedChanges          []string
	UnknownChanges           []string
}

func Verify(agent string, observed ObservedRuntime) IntegrityReport {
	contract := DefaultContract()
	report := NewIntegrityReport(agent, contract)

	verifyManifest(&report, contract, observed.Manifest)
	verifyPromptInvariants(&report, observed)
	verifyStoreInvariants(&report, observed)
	verifyRegistryInvariant(&report, contract, observed.RegistryPath)
	verifyMemoryTopicInvariants(&report, observed)
	verifyModelInvariants(&report, contract, observed)
	verifyManagedArtifacts(&report, contract, observed.Artifacts)
	verifyNonOwnedDrift(&report, observed.NonOwnedChanges)
	verifyUnknownDrift(&report, observed.UnknownChanges)

	return report
}

func verifyPromptInvariants(report *IntegrityReport, observed ObservedRuntime) {
	expectedIDs, err := DefaultPromptSourceIDs(report.Agent, "orchestrator")
	if err != nil {
		report.AddCheck(CheckResult{Key: "invariant.prompt.required_sources_order", Status: StatusFail, DriftClass: DriftOwned, Expected: "canonical ordered required source ids", Observed: "prompt contract resolution error", Message: "failed to resolve canonical prompt contract"})
		return
	}

	status := StatusPass
	message := "prompt source composition invariant matches canonical required ordering"
	if strings.Join(observed.PromptSourceIDs, "|") != strings.Join(expectedIDs, "|") {
		status = StatusFail
		message = "prompt source composition drift detected (missing/extra/reordered sources)"
	}

	report.AddCheck(CheckResult{Key: "invariant.prompt.required_sources_order", Status: status, DriftClass: driftClassFromStatus(status), Expected: strings.Join(expectedIDs, ","), Observed: strings.Join(observed.PromptSourceIDs, ","), Message: message})
}

func verifyStoreInvariants(report *IntegrityReport, observed ObservedRuntime) {
	resolved, err := ResolveStoreContract(observed.StoreMode)
	if err != nil {
		report.AddCheck(CheckResult{Key: "invariant.store.mode", Status: StatusFail, DriftClass: DriftOwned, Expected: "hive|openspec|hybrid", Observed: observed.StoreMode, Message: "store mode drift: unsupported mode"})
		return
	}

	readStatus := StatusPass
	readMsg := "store contract read targets match mode contract"
	if strings.Join(observed.StoreReadFrom, "|") != strings.Join(resolved.ReadFrom, "|") {
		readStatus = StatusFail
		readMsg = "store contract read-target drift detected"
	}
	report.AddCheck(CheckResult{Key: "invariant.store.read_targets", Status: readStatus, DriftClass: driftClassFromStatus(readStatus), Expected: strings.Join(resolved.ReadFrom, ","), Observed: strings.Join(observed.StoreReadFrom, ","), Message: readMsg})

	writeStatus := StatusPass
	writeMsg := "store contract write targets match mode contract"
	if strings.Join(observed.StoreWriteTo, "|") != strings.Join(resolved.WriteTo, "|") {
		writeStatus = StatusFail
		writeMsg = "store contract write-target drift detected"
	}
	report.AddCheck(CheckResult{Key: "invariant.store.write_targets", Status: writeStatus, DriftClass: driftClassFromStatus(writeStatus), Expected: strings.Join(resolved.WriteTo, ","), Observed: strings.Join(observed.StoreWriteTo, ","), Message: writeMsg})

	report.AddCheck(CheckResult{Key: "invariant.store.mode", Status: StatusPass, DriftClass: DriftNone, Expected: "hive|openspec|hybrid", Observed: string(resolved.Mode), Message: "store mode invariant accepted"})
}

func verifyMemoryTopicInvariants(report *IntegrityReport, observed ObservedRuntime) {
	artifactStatus := StatusPass
	artifactObserved := "all topics valid"
	for _, topic := range observed.ArtifactTopics {
		if !IsSDDArtifactTopic(topic) {
			artifactStatus = StatusFail
			artifactObserved = topic
			break
		}
	}
	report.AddCheck(CheckResult{Key: "invariant.memory.artifact_topics_boundary", Status: artifactStatus, DriftClass: driftClassFromStatus(artifactStatus), Expected: "sdd/{change}/{artifact}", Observed: artifactObserved, Message: "artifact memory topics must stay within SDD artifact topic boundary"})

	generalStatus := StatusPass
	generalObserved := "no sdd/* topic leakage"
	for _, topic := range observed.GeneralMemoryTopics {
		if IsSDDArtifactTopic(topic) || strings.HasPrefix(topic, "sdd/") {
			generalStatus = StatusFail
			generalObserved = topic
			break
		}
	}
	report.AddCheck(CheckResult{Key: "invariant.memory.general_topics_boundary", Status: generalStatus, DriftClass: driftClassFromStatus(generalStatus), Expected: "non-sdd topics", Observed: generalObserved, Message: "general memory topics must not reuse reserved sdd artifact namespace"})
}

func verifyUnknownDrift(report *IntegrityReport, notes []string) {
	if len(notes) == 0 {
		return
	}

	report.AddCheck(CheckResult{
		Key:        "drift.unknown",
		Status:     StatusWarn,
		DriftClass: DriftUnknown,
		Expected:   "no unknown changes",
		Observed:   "unknown changes detected",
		Message:    "unknown changes detected; manual inspection required",
	})
	report.Notes = append(report.Notes, notes...)
}

func verifyManifest(report *IntegrityReport, contract Contract, manifest RuntimeManifestState) {
	if !manifest.Present {
		report.AddCheck(CheckResult{
			Key:        "manifest.present",
			Status:     StatusFail,
			DriftClass: DriftOwned,
			Expected:   "present",
			Observed:   "missing",
			Message:    "runtime manifest missing; rerun setup/repair to restore managed runtime state",
		})
		return
	}

	if manifest.Corrupted {
		report.AddCheck(CheckResult{
			Key:        "manifest.integrity",
			Status:     StatusFail,
			DriftClass: DriftOwned,
			Expected:   "valid",
			Observed:   "corrupted",
			Message:    "runtime manifest corrupted; rerun setup/repair to regenerate managed runtime state",
		})
		return
	}

	status := StatusPass
	message := "runtime manifest present"
	if manifest.ContractVersion != "" && manifest.ContractVersion != contract.Version {
		status = StatusFail
		message = "runtime manifest contract version mismatch; rerun setup/repair"
	}

	report.AddCheck(CheckResult{
		Key:        "manifest.contract_version",
		Status:     status,
		DriftClass: driftClassFromStatus(status),
		Expected:   contract.Version,
		Observed:   manifest.ContractVersion,
		Message:    message,
	})

	verifyManagedArtifactCatalog(report, contract, manifest.ManagedArtifactIDs)
}

func verifyManagedArtifactCatalog(report *IntegrityReport, contract Contract, observedIDs []string) {
	expectedIDs := requiredManagedArtifactIDs(contract.ManagedArtifacts)
	observedSet := make(map[string]struct{}, len(observedIDs))
	for _, id := range observedIDs {
		observedSet[id] = struct{}{}
	}

	missing := 0
	for _, id := range expectedIDs {
		if _, ok := observedSet[id]; !ok {
			missing++
		}
	}

	status := StatusPass
	message := "runtime manifest managed artifact catalog complete"
	if missing > 0 {
		status = StatusFail
		message = "runtime manifest managed artifact catalog incomplete; rerun setup/repair"
	}

	report.AddCheck(CheckResult{
		Key:        "manifest.managed_artifacts",
		Status:     status,
		DriftClass: driftClassFromStatus(status),
		Expected:   fmt.Sprintf("%d/%d", len(expectedIDs), len(expectedIDs)),
		Observed:   fmt.Sprintf("%d/%d", len(observedSet), len(expectedIDs)),
		Message:    message,
	})
}

func requiredManagedArtifactIDs(artifacts []ManagedArtifact) []string {
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Required {
			ids = append(ids, artifact.ID)
		}
	}
	return ids
}

func managedArtifactIDs(artifacts []ManagedArtifact) []string {
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		ids = append(ids, artifact.ID)
	}
	return ids
}

func verifyRegistryInvariant(report *IntegrityReport, contract Contract, observed string) {
	status := StatusPass
	message := "registry path invariant matches contract"
	if observed != contract.RegistryPath {
		status = StatusFail
		message = "registry path mismatch for contract-owned invariant"
	}

	report.AddCheck(CheckResult{
		Key:        "invariant.registry_path",
		Status:     status,
		DriftClass: driftClassFromStatus(status),
		Expected:   contract.RegistryPath,
		Observed:   observed,
		Message:    message,
	})
}

func verifyModelInvariants(report *IntegrityReport, contract Contract, observed ObservedRuntime) {
	platform, err := platformForAgent(report.Agent)
	if err != nil {
		report.AddCheck(CheckResult{
			Key:        "invariant.model.platform",
			Status:     StatusFail,
			DriftClass: DriftOwned,
			Expected:   "supported platform",
			Observed:   report.Agent,
			Message:    "unsupported agent platform for model assignment verification",
		})
		return
	}

	expectedAssignments, err := DefaultAssignmentsForPlatform(platform)
	if err != nil {
		report.AddCheck(CheckResult{
			Key:        "invariant.model.platform",
			Status:     StatusFail,
			DriftClass: DriftOwned,
			Expected:   "supported platform",
			Observed:   string(platform),
			Message:    "unable to derive platform default model assignments",
		})
		return
	}

	for _, phase := range contract.Phases {
		expected, ok := expectedAssignments[phase]
		if !ok {
			continue
		}

		observedValue := observed.ModelAssignments[phase]
		status := StatusPass
		message := fmt.Sprintf("model assignment invariant matches for %s", phase)
		if observedValue != expected {
			status = StatusFail
			message = fmt.Sprintf("model assignment mismatch for contract-owned phase %s", phase)
		}

		report.AddCheck(CheckResult{
			Key:        fmt.Sprintf("invariant.model.%s", phase),
			Status:     status,
			DriftClass: driftClassFromStatus(status),
			Expected:   expected,
			Observed:   observedValue,
			Message:    message,
		})
	}
}

func verifyManagedArtifacts(report *IntegrityReport, contract Contract, observed map[string]ObservedArtifact) {
	for _, artifact := range contract.ManagedArtifacts {
		entry := observed[artifact.ID]
		if !artifact.Required && !entry.Exists {
			continue
		}
		status := StatusPass
		drift := DriftNone
		message := "managed artifact present"

		if !entry.Exists {
			status = StatusFail
			drift = DriftOwned
			message = "required managed artifact missing"
		} else if artifact.Scope == OwnershipBlock && !entry.MarkersValid {
			status = StatusFail
			drift = DriftOwned
			message = "managed artifact markers missing or out of boundary"
		}

		report.AddCheck(CheckResult{
			Key:        fmt.Sprintf("artifact.%s.present", artifact.ID),
			Status:     status,
			DriftClass: drift,
			Expected:   "present",
			Observed:   observedLabel(entry.Exists),
			Message:    message,
		})
	}
}

func verifyNonOwnedDrift(report *IntegrityReport, notes []string) {
	if len(notes) == 0 {
		return
	}

	report.AddCheck(CheckResult{
		Key:        "drift.non_owned",
		Status:     StatusWarn,
		DriftClass: DriftNonOwned,
		Expected:   "no non-owned changes",
		Observed:   "user-owned changes detected",
		Message:    "non-owned changes detected outside managed boundaries",
	})
	report.Notes = append(report.Notes, notes...)
}

func driftClassFromStatus(status IntegrityStatus) DriftClass {
	if status == StatusFail {
		return DriftOwned
	}
	return DriftNone
}

func observedLabel(exists bool) string {
	if exists {
		return "present"
	}
	return "missing"
}
