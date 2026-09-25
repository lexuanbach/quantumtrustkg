// Reconciler for AssignmentStatus objects and the one-shot entry point
// (Sect. 3, implementation).
//
// AssignmentStatus is the controller-owned record of one edge: desired profile,
// active profile, state and reason. Reconciling one re-evaluates only the edge it
// names (sourceService and destinationService in its spec), which is the
// incremental behaviour the paper describes. When an object carries no usable
// target the reconciler falls back to the whole fixture or, in a live cluster, to
// the whole namespace.
//
// ReconcileNamed is the non-Kubernetes entry used by cmd/manager --fixture and by
// the tests. It runs the same runtime path once without a manager.

package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AssignmentReconciler watches AssignmentStatus resources and also serves as
// the one-shot reconciler behind ReconcileNamed.
type AssignmentReconciler struct {
	client.Client
	Runtime      *AssignmentRuntime
	WatchFixture string
}

// Reconcile re-evaluates the edge named by the object. With WatchFixture empty
// the whole namespace is rebuilt from the cluster inventory. The status
// subresource is written by the runtime, which means this method never edits the object
// it reads.
func (r *AssignmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.Runtime == nil {
		return ctrl.Result{}, nil
	}
	obj := assignmentObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	source, destination, err := parseAssignmentTarget(obj)
	if err == nil {
		if r.WatchFixture == "" {
			return resultFor(r.Runtime.ReconcileClusterNamespace(ctx, req.Namespace))
		}
		return resultForEdge(r.Runtime.ReconcileFixtureEdgeByServices(ctx, r.WatchFixture, source, destination))
	}
	if r.WatchFixture == "" || r.WatchFixture == "demo" {
		return resultForEdge(r.Runtime.ReconcileDemoEdge(ctx))
	}
	return resultFor(r.Runtime.ReconcileFixtureEdges(ctx, r.WatchFixture))
}

// ReconcileNamed evaluates a fixture once. An empty name or "demo" selects the
// finance demo edge (authToPayment). Any other name is a fixture file under
// implementation/semantics/fixtures.
func (r *AssignmentReconciler) ReconcileNamed(name string) error {
	if r.Runtime == nil {
		return nil
	}
	if name == "" || name == "demo" {
		_, err := r.Runtime.ReconcileDemoEdge(context.Background())
		return err
	}
	_, err := r.Runtime.ReconcileFixtureEdges(context.Background(), name)
	return err
}

// SetupWithManager registers the reconciler for the AssignmentStatus kind.
func (r *AssignmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(assignmentObject()).
		Complete(r)
}
