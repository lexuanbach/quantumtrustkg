// Controller manager wiring for QuantumTrustKG (Sect. 3, implementation).
//
// StartManager builds one controller-runtime manager, creates a single shared
// AssignmentRuntime and registers four reconcilers on it. The three operator CRDs
// (QuantumSecurityPolicy, CryptoCapabilityProfile, MigrationPlan) feed the
// in-memory ResourceState, and the controller-owned AssignmentStatus reconciler
// re-evaluates a named edge. Every reconciler calls into the same runtime, which means a
// change to any input triggers the same per-edge QTPO evaluation.
//
// Leader election is enabled. Only the lease holder reconciles and writes status,
// which keeps two replicas from racing on the same AssignmentStatus objects. The
// runtime is started with FailClosedOnGraphUnavailable set, which is the G4
// behaviour of Thm. 2: if the Fuseki graph cannot be reached, edges are blocked
// and keep their last committed profile.

package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"quantumtrustkg/controller/internal/mesh"
)

// ManagerConfig carries the deployment settings of the manager. WatchFixture
// names a fixture under implementation/semantics/fixtures. When it is empty the
// reconcilers build edges from the cluster (Services annotated with
// quantumtrustkg.io/calls). When it is set they reconcile the fixture's edges
// against the CRD-supplied policies and capabilities, which is what the
// in-cluster evaluation harness uses.
type ManagerConfig struct {
	GraphEndpoint string
	AssetsRoot    string
	WatchFixture  string
	MetricsAddr   string
	ProbeAddr     string
}

// StartManager runs the controller until the process receives a termination
// signal. It returns an error if the manager, one of the reconcilers or the
// health endpoints cannot be set up.
func StartManager(cfg ManagerConfig) error {
	options := ctrl.Options{
		Metrics:                ctrl.Options{}.Metrics,
		HealthProbeBindAddress: cfg.ProbeAddr,
		// Leader election serializes reconciliation across controller
		// replicas: only the lease holder reconciles and writes status.
		LeaderElection:          true,
		LeaderElectionID:        "quantumtrustkg-controller.quantumtrustkg.io",
		LeaderElectionNamespace: "quantumtrustkg-system",
	}
	options.Metrics.BindAddress = cfg.MetricsAddr

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		return fmt.Errorf("create controller manager: %w", err)
	}

	runtime := NewAssignmentRuntime(cfg.GraphEndpoint, cfg.AssetsRoot)
	runtime.Inventory = NewClusterInventorySource(mgr.GetClient(), runtime.Resources)
	runtime.StatusWriter = NewKubernetesStatusWriter(mgr.GetClient())
	runtime.MigrationWriter = NewKubernetesStatusWriter(mgr.GetClient())
	runtime.HealthObserver = mesh.NewClusterHealthObserver(mgr.GetClient())
	runtime.FailClosedOnGraphUnavailable = true

	if err := (&PolicyReconciler{
		Client:       mgr.GetClient(),
		Runtime:      runtime,
		WatchFixture: cfg.WatchFixture,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup policy reconciler: %w", err)
	}

	if err := (&CapabilityReconciler{
		Client:       mgr.GetClient(),
		Runtime:      runtime,
		WatchFixture: cfg.WatchFixture,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup capability reconciler: %w", err)
	}

	if err := (&MigrationReconciler{
		Client:       mgr.GetClient(),
		Runtime:      runtime,
		WatchFixture: cfg.WatchFixture,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup migration reconciler: %w", err)
	}

	if err := (&AssignmentReconciler{
		Client:       mgr.GetClient(),
		Runtime:      runtime,
		WatchFixture: cfg.WatchFixture,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup assignment reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}
