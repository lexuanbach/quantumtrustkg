// Reconciler for QuantumSecurityPolicy objects (Sect. 3, implementation).
//
// A policy object states the required security class for the edge set selected by
// its labels (the Req(e) input of gate (i) of the QTPO filter, Sect. 3). The reconciler parses
// the object, stores it in the shared ResourceState and then re-evaluates the
// edges, either from the cluster inventory or from a named fixture. It does not
// decide anything itself. Selection, promotion and status writing happen in
// AssignmentRuntime.

package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/internal/observability"
)

// PolicyReconciler watches QuantumSecurityPolicy resources. WatchFixture is
// empty for a live cluster and names a fixture for the fixture-backed runs.
type PolicyReconciler struct {
	client.Client
	Runtime      *AssignmentRuntime
	WatchFixture string
}

// Reconcile handles one policy event. A deleted policy is ignored here, which means it
// stays in ResourceState until the process restarts. Limitation: policy
// withdrawal is not propagated.
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	observability.LogReconcileStart("policy:" + req.NamespacedName.String())
	if r.Runtime == nil {
		return ctrl.Result{}, nil
	}
	obj := policyObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	policy, err := parsePolicy(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Runtime.Resources.UpsertPolicy(policy)
	if r.WatchFixture == "" {
		return resultFor(r.Runtime.ReconcileClusterNamespace(ctx, req.Namespace))
	}
	return resultFor(r.Runtime.ReconcileFixtureEdges(ctx, r.WatchFixture))
}

// SetupWithManager registers the reconciler for the policy kind.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(policyObject()).
		Complete(r)
}
