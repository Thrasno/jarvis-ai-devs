package lifecycle

import (
	"fmt"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

type Ledger struct {
	Version               string `json:"version"`
	JarvisVersion         string `json:"jarvis_version"`
	ContractVersion       string `json:"contract_version"`
	ProviderSchemaVersion string `json:"provider_schema_version"`
}

type ObservedProviderState struct {
	Artifacts       map[string]sddruntime.ObservedArtifact
	NonOwnedChanges []string
	UnknownChanges  []string
}

type VerifyResult struct {
	Status sddruntime.IntegrityStatus
	Report sddruntime.IntegrityReport
}

type DoctorStep struct {
	AssetID      string
	SafetyClass  string
	BackupNeeded bool
}

type DoctorPlan struct {
	ReadOnly bool
	Steps    []DoctorStep
}

type LifecycleError struct {
	Code       string
	AssetID    string
	Scope      string
	Stage      string
	NextAction string
	Err        error
}

func NewLifecycleError(code, assetID, scope, stage, nextAction string, err error) *LifecycleError {
	return &LifecycleError{Code: code, AssetID: assetID, Scope: scope, Stage: stage, NextAction: nextAction, Err: err}
}

func (e *LifecycleError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s (asset=%s scope=%s stage=%s next_action=%s)", e.Code, e.AssetID, e.Scope, e.Stage, e.NextAction)
	}
	return fmt.Sprintf("%s: %v (asset=%s scope=%s stage=%s next_action=%s)", e.Code, e.Err, e.AssetID, e.Scope, e.Stage, e.NextAction)
}

func (e *LifecycleError) Unwrap() error { return e.Err }

type ReconcileResult struct {
	Applied         int
	SkippedNonOwned []string
}

type RestoreResult struct {
	Restored int
}

type UninstallResult struct {
	Applied       int
	VerifyStatus  sddruntime.IntegrityStatus
	LedgerRemoved bool
}

type BackupTarget struct {
	Path string `json:"path"`
}

type BackupEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}

type BackupManifest struct {
	SnapshotID            string        `json:"snapshot_id"`
	SourceOperation       string        `json:"source_operation"`
	CreatedAt             string        `json:"created_at"`
	JarvisVersion         string        `json:"jarvis_version"`
	ContractVersion       string        `json:"contract_version"`
	ProviderSchemaVersion string        `json:"provider_schema_version"`
	ArchivePath           string        `json:"archive_path"`
	Entries               []BackupEntry `json:"entries"`
}
