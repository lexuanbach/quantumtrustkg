// Reconcile logging (Sect. 3, implementation).
//
// LogReconcileStart marks the start of a reconciliation for a named object or
// edge. The name carries the kind prefix used by the reconcilers ("policy:",
// "capability:", "migration:") or the edge ID.

package observability

import "log"

// LogReconcileStart logs the start of a reconciliation.
func LogReconcileStart(name string) {
	log.Printf("reconcile_start=%s\n", name)
}
