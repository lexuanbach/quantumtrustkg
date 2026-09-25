// Command manager is the entry point of the QuantumTrustKG controller
// (Sect. 3, implementation).
//
// Without flags it starts the controller-runtime manager (controllers.StartManager),
// which watches the three operator CRDs and AssignmentStatus and reconciles them
// with QTPO. With --fixture NAME it does one non-Kubernetes pass over the fixture
// NAME in implementation/semantics/fixtures and exits. This mode is how
// experiments/scripts/run-fixture.sh drives the pipeline without a cluster.
//
// The graph endpoint, namespace and probe and metrics addresses are constants in
// this file (http://fuseki:3030/quantumtrustkg, quantumtrustkg-system, :8081 and
// :8080). Limitation: only --fixture, --watch-fixture and --manager are flags.

package main

import (
	"flag"
	"log"
	"path/filepath"

	"quantumtrustkg/controller/internal/controllers"
)

// main parses the flags and dispatches between the one-shot and manager modes.
func main() {
	log.Println("QuantumTrustKG controller bootstrap starting")

	var fixture string
	var watchFixture string
	var managerMode bool
	flag.StringVar(&fixture, "fixture", "", "fixture file to reconcile once from implementation/semantics/fixtures")
	flag.StringVar(&watchFixture, "watch-fixture", "finance-scenario.ttl", "fixture file used by watched reconciliations until CRD ingestion is wired in")
	flag.BoolVar(&managerMode, "manager", true, "start the controller-runtime manager instead of one-shot fixture reconciliation")
	flag.Parse()

	// Fixed deployment settings. The namespace is informational here, because
	// leader election names its own namespace in controllers.StartManager.
	config := struct {
		Namespace      string
		GraphEndpoint  string
		MetricsAddress string
		ProbeAddress   string
		AssetsRoot     string
	}{
		Namespace:      "quantumtrustkg-system",
		GraphEndpoint:  "http://fuseki:3030/quantumtrustkg",
		MetricsAddress: ":8080",
		ProbeAddress:   ":8081",
		AssetsRoot:     filepath.Clean(".."),
	}

	if fixture != "" {
		runtime := controllers.NewAssignmentRuntime(
			config.GraphEndpoint,
			config.AssetsRoot,
		)
		reconciler := controllers.AssignmentReconciler{Runtime: runtime}

		if err := reconciler.ReconcileNamed(fixture); err != nil {
			log.Printf("reconcile failed: %v\n", err)
		} else {
			log.Printf("one-shot reconciliation completed fixture=%s namespace=%s metrics=%s\n",
				fixture,
				config.Namespace,
				config.MetricsAddress,
			)
		}
		return
	}

	if managerMode {
		if err := controllers.StartManager(controllers.ManagerConfig{
			GraphEndpoint: config.GraphEndpoint,
			AssetsRoot:    config.AssetsRoot,
			WatchFixture:  watchFixture,
			MetricsAddr:   config.MetricsAddress,
			ProbeAddr:     config.ProbeAddress,
		}); err != nil {
			log.Fatalf("manager failed: %v", err)
		}
	}
}
