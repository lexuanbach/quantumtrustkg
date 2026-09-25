// Smoke test of the fixture-mode runtime (Sect. 3, Fig. 1 loop).
//
// TestReconcileDemoEdgeProducesAssignment runs the whole loop once on the finance
// demo edge with the default filesystem applier. It checks that a desired and an
// active profile are set and that the edge ends Promoted. The runtime uses an
// unreachable graph endpoint, and FailClosedOnGraphUnavailable is off, which means this is
// the fixture path without a graph. Limitation: the default applier writes its
// generated manifests under the assets root, which for this test is the
// implementation directory.

package unit

import (
	"context"
	"path/filepath"
	"testing"

	"quantumtrustkg/controller/internal/controllers"
	"quantumtrustkg/controller/pkg/types"
)

// TestReconcileDemoEdgeProducesAssignment is the end-to-end smoke test.
func TestReconcileDemoEdgeProducesAssignment(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)

	edge, err := runtime.ReconcileDemoEdge(context.Background())
	if err != nil {
		t.Fatalf("expected reconciliation to succeed, got %v", err)
	}
	if edge.DesiredProfile == "" {
		t.Fatal("expected desired profile to be set")
	}
	if edge.ActiveProfile == "" {
		t.Fatal("expected active profile to be set")
	}
	if edge.State != types.AssignmentPromoted {
		t.Fatalf("expected promoted state, got %s", edge.State)
	}
}
