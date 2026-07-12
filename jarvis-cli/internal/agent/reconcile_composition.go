package agent

import (
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

// NativeMCPReplacer is the narrow native reconciliation boundary used by the
// installer composition. Implementations keep user-global scope semantics.
type NativeMCPReplacer interface {
	Replace([]NativeMCPDefinition) (*NativeMCPResult, error)
}

// ReconcileInstallRequest contains the already-rendered Jarvis/Hive Store plan
// and native MCP desired state. Callers provide the evidence location so this
// composition never resolves or writes real user configuration paths itself.
type ReconcileInstallRequest struct {
	Store        reconcile.CompensationStore
	StorePlan    reconcile.Plan
	EvidencePath string
	DesiredMCPs  []NativeMCPDefinition
}

// ReconcileInstallResult is secret-safe composition evidence. Native results
// intentionally remain transient; degraded Store evidence is durable.
type ReconcileInstallResult struct {
	Store  reconcile.ApplyReport
	Native NativeMCPResult
}

// ReconcileInstall applies Jarvis/Hive Store reconciliation before the optional
// user-global native MCP step. Empty desired native state is an explicit skip,
// allowing no-agent callers to complete Store work without native commands.
func ReconcileInstall(request ReconcileInstallRequest, native NativeMCPReplacer) (ReconcileInstallResult, error) {
	evidence, err := reconcile.NewFileRecoveryEvidenceStore(request.EvidencePath)
	if err != nil {
		return ReconcileInstallResult{}, errors.New("recovery evidence adapter is unavailable; repair its location and rerun Install/Reconfigure")
	}
	if request.Store == nil {
		return ReconcileInstallResult{}, errors.New("Jarvis/Hive Store adapter is unavailable; repair it and rerun Install/Reconfigure")
	}

	storeReport, err := reconcile.ApplyWithCompensation(request.Store, evidence, request.StorePlan)
	result := ReconcileInstallResult{Store: storeReport}
	if err != nil {
		return result, err
	}
	if len(request.DesiredMCPs) == 0 {
		result.Native = NativeMCPResult{Phase: NativeMCPSkipped}
		return result, nil
	}
	if native == nil {
		return result, errors.New("native MCP adapter is unavailable; correct the native MCP error and rerun Install/Reconfigure")
	}

	nativeResult, err := native.Replace(request.DesiredMCPs)
	if nativeResult != nil {
		result.Native = *nativeResult
	}
	if err != nil {
		return result, fmt.Errorf("native MCP reconciliation failed (%s): %s", result.Native.Diagnostics(), nativeMCPFixForwardGuidance)
	}
	return result, nil
}
