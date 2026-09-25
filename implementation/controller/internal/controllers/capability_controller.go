// Reconciler for CryptoCapabilityProfile objects (Sect. 3, implementation).
//
// A capability object records which profiles a workload can speak, together with
// its source, an observation time and a freshness window (300 s when the object
// gives none). Together these are the provenance fields that gate (iv) of the
// QTPO filter checks. The reconciler stores the parsed record in ResourceState and
// re-evaluates the edges. Whether the record is still fresh is decided at
// evaluation time in AssignmentRuntime.capabilityEvidence and is not decided here.

package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/internal/observability"
)

// CapabilityReconciler watches CryptoCapabilityProfile resources.
type CapabilityReconciler struct {
	client.Client
	Runtime      *AssignmentRuntime
	WatchFixture string
}

// Reconcile handles one capability event. A parse failure is returned as an
// error, which means controller-runtime retries with backoff and the record never enters
// ResourceState. An unparsable record therefore cannot widen an endpoint's
// advertised set.
func (r *CapabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	observability.LogReconcileStart("capability:" + req.NamespacedName.String())
	if r.Runtime == nil {
		return ctrl.Result{}, nil
	}
	obj := capabilityObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	capability, err := parseCapability(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Runtime.Resources.UpsertCapability(capability)
	if r.WatchFixture == "" {
		return resultFor(r.Runtime.ReconcileClusterNamespace(ctx, req.Namespace))
	}
	return resultFor(r.Runtime.ReconcileFixtureEdges(ctx, r.WatchFixture))
}

// SetupWithManager registers the reconciler for the capability kind.
func (r *CapabilityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(capabilityObject()).
		Complete(r)
}
