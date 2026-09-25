// Package state holds the controller's in-memory caches of edges and of
// assignment outcomes, and two small helpers on assignment state and provenance.
// The Kubernetes API is the source of truth (Sect. 3, Implementation), and these
// caches only avoid re-reading it inside one reconciliation loop. Nothing here
// takes a lock. A store must be used from one goroutine at a time.
package state

import "quantumtrustkg/controller/pkg/types"

// TransitionAssignment returns a copy of the edge with State and Reason set. It
// does not check that the move from the old state to the new one is legal.
// Callers use it to record the outcomes Blocked, Applying, Promoted and Failed
// of the assignment lifecycle in types.AssignmentState.
func TransitionAssignment(edge types.CommunicationEdge, next types.AssignmentState, reason string) types.CommunicationEdge {
	edge.State = next
	edge.Reason = reason
	return edge
}
