// Reconciler for MigrationPlan objects (Sect. 3, implementation).
//
// A MigrationPlan selects the edges to migrate and gives the rollout strategy, the
// maximum number of unavailable edges and whether to roll back on error. This
// reconciler only records the plan in ResourceState and triggers a new
// evaluation. The staged windows themselves come from
// orchestration.ComputeProgressiveRolloutWindow, which the runtime calls, and the
// plan's status (phase, completed edges, last error) is written back by
// KubernetesStatusWriter. Rollout feasibility is the fifth gate of the QTPO
// filter, which means a plan can defer an edge but cannot make an inadmissible profile
// admissible.

package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/internal/observability"
)

// MigrationReconciler watches MigrationPlan resources.
type MigrationReconciler struct {
	client.Client
	Runtime      *AssignmentRuntime
	WatchFixture string
}

// Reconcile handles one MigrationPlan event and re-evaluates the edges.
func (r *MigrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	observability.LogReconcileStart("migration:" + req.NamespacedName.String())
	if r.Runtime == nil {
		return ctrl.Result{}, nil
	}
	obj := migrationObject()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	migration, err := parseMigration(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Runtime.Resources.UpsertMigration(migration)
	if r.WatchFixture == "" {
		return resultFor(r.Runtime.ReconcileClusterNamespace(ctx, req.Namespace))
	}
	return resultFor(r.Runtime.ReconcileFixtureEdges(ctx, r.WatchFixture))
}

// SetupWithManager registers the reconciler for the migration kind.
func (r *MigrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(migrationObject()).
		Complete(r)
}
