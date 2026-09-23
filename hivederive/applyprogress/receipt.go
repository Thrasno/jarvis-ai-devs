package applyprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// AdvanceReceiptDigest computes the daemon's persisted advance receipt identity.
// The JSON field order and byte-slice encoding are part of the historical format.
func AdvanceReceiptDigest(project, change, requestID string, generation, revision uint64, expectedDigest, legacySource string, snapshot []byte, batches [][]byte) string {
	payload, _ := json.Marshal(struct {
		Project            string   `json:"project"`
		Change             string   `json:"change"`
		RequestID          string   `json:"request_id"`
		Generation         uint64   `json:"expected_generation"`
		Revision           uint64   `json:"expected_revision"`
		Digest             string   `json:"expected_digest"`
		LegacySourceSHA256 string   `json:"legacy_source_sha256,omitempty"`
		Snapshot           []byte   `json:"snapshot"`
		Batches            [][]byte `json:"batches"`
	}{project, change, requestID, generation, revision, expectedDigest, legacySource, snapshot, batches})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
