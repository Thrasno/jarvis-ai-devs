package agent

import "github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"

// InventoryProvider is the optional platform seam used by reconciliation to
// observe adapter-specific artifacts without granting reconciliation mutation
// access to an Agent. Implementations must classify nothing: ownership proof
// remains the reconcile package's responsibility.
type InventoryProvider interface {
	Inventory() (reconcile.Inventory, error)
}
