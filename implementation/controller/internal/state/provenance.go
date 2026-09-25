package state

import (
	"time"

	"quantumtrustkg/controller/pkg/types"
)

// IsAdmissible is the freshness check of evidence gate (iv) on the edge
// provenance. It is true when a freshness deadline is recorded and now is not
// after it. The deadline instant itself still counts as fresh. A zero
// FreshUntil is treated as unknown freshness and fails, which is the
// fail-closed choice. The check does not look at ConflictNote. The runtime
// applies graph.ValidateEdge, which adds the contradiction test.
func IsAdmissible(p types.EdgeProvenance, now time.Time) bool {
	if p.FreshUntil.IsZero() {
		return false
	}
	return !now.After(p.FreshUntil)
}
