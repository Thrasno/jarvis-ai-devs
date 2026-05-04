package sddruntime

import "fmt"

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
	verifyRegistryInvariant(&report, contract, observed.RegistryPath)
	verifyModelInvariants(&report, contract, observed)
	verifyManagedArtifacts(&report, contract, observed.Artifacts)
	verifyNonOwnedDrift(&report, observed.NonOwnedChanges)
	verifyUnknownDrift(&report, observed.UnknownChanges)

	return report
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
