package sync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const managedAssetDigestVersion = "jarvis-sync-assets-v1"

// ManagedAssetDigest identifies the exact desired asset set represented by a
// plan. Length framing prevents field-boundary ambiguity and sorting makes the
// identity independent of planner traversal order.
func ManagedAssetDigest(plan Plan) string {
	tracked := append([]TrackedPath(nil), plan.Tracked...)
	sort.Slice(tracked, func(i, j int) bool {
		if tracked[i].Agent != tracked[j].Agent {
			return tracked[i].Agent < tracked[j].Agent
		}
		return tracked[i].Identity < tracked[j].Identity
	})
	h := sha256.New()
	writeDigestField(h, []byte(managedAssetDigestVersion))
	for _, item := range tracked {
		writeDigestField(h, []byte(item.Agent))
		writeDigestField(h, []byte(item.Identity))
		writeDigestField(h, []byte(item.Mode.String()))
		writeDigestField(h, []byte(item.Desired))
		semantic, _ := json.Marshal(item.Semantic)
		writeDigestField(h, semantic)
	}
	return managedAssetDigestVersion + ":" + hex.EncodeToString(h.Sum(nil))
}

type digestWriter interface{ Write([]byte) (int, error) }

func writeDigestField(w digestWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write(value)
}
