package controllers

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"quantumtrustkg/controller/pkg/types"
)

// Requeue policy for the staged-promotion path of Sect. 3 (implementation).
// A promotion is recorded as active only after the PeerAuthentication and
// DestinationRule exist and both workloads are Ready. Until then an edge sits
// in the Applying state, and the reconciler asks controller-runtime to visit it
// again. This file holds the small helpers that turn edge states into that
// requeue decision.

// readinessRequeue is how long the controller waits before re-checking edges
// whose mesh objects are applied but whose workloads are not yet Ready.
const readinessRequeue = 5 * time.Second

// resultFor converts the outcome of a reconciliation into a controller-runtime
// result. Errors are passed through. Otherwise the next visit is the earlier of
// the readiness poll and the first future evidence-expiry deadline. Scheduling
// expiry explicitly is necessary because the passage of time does not itself
// produce a Kubernetes watch event.
func resultFor(edges []types.CommunicationEdge, err error) (ctrl.Result, error) {
	if err != nil {
		return ctrl.Result{}, err
	}
	return resultForAt(edges, time.Now().UTC()), nil
}

func resultForAt(edges []types.CommunicationEdge, now time.Time) ctrl.Result {
	var next time.Duration
	for _, edge := range edges {
		if edge.State == types.AssignmentApplying {
			next = readinessRequeue
		}
		if deadline := edge.Provenance.FreshUntil; deadline.After(now) {
			until := deadline.Sub(now)
			if next == 0 || until < next {
				next = until
			}
		}
	}
	return ctrl.Result{RequeueAfter: next}
}

// resultForEdge is resultFor for a single edge.
func resultForEdge(edge types.CommunicationEdge, err error) (ctrl.Result, error) {
	return resultFor([]types.CommunicationEdge{edge}, err)
}
