package applyprogress

// BuildSuccessorGenesis constructs the canonical fresh successor from an authenticated
// signed predecessor seal. Callers must independently verify stored authority and
// the revised authoritative tasks before publishing the returned bytes.
func BuildSuccessorGenesis(predecessor Snapshot) (Snapshot, []byte, error) {
	if predecessor.Schema != SupersessionSnapshotSchema || predecessor.Status != StatusSuperseded || predecessor.SealIntent == nil || VerifySnapshot(predecessor) != nil {
		return Snapshot{}, nil, invalid(CodeInvalidBase, "predecessor seal")
	}
	intent := predecessor.SealIntent
	pointer := &SupersedesPointer{Project: predecessor.Project, Change: predecessor.Change, SealDigest: predecessor.Digest, OriginalManifestSHA256: predecessor.TaskManifestSHA256, Actor: intent.Actor, Reason: intent.Reason, Timestamp: intent.Timestamp, OperationID: intent.OperationID}
	genesis, data, err := SealSnapshot(Snapshot{Schema: SupersessionSnapshotSchema, Project: intent.SuccessorProject, Change: intent.SuccessorChange, Generation: 1, Revision: 1, TaskManifestSHA256: intent.SuccessorManifestSHA256, Status: StatusPartial, Coverage: []Coverage{}, Batches: []BatchRef{}, Supersedes: pointer})
	if err != nil {
		return Snapshot{}, nil, err
	}
	if err := ValidateSuccessorGenesisPair(predecessor, genesis); err != nil {
		return Snapshot{}, nil, err
	}
	return genesis, data, nil
}
