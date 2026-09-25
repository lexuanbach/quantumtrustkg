// Test of the MigrationPlan status write-back (Sect. 3, staged migration).
//
// TestKubernetesStatusWriterUpdatesMigrationPlanStatus stores a plan status through
// KubernetesStatusWriter into a fake API server and reads back the phase and the
// completed-edge count. These are the fields that let an operator follow a staged
// rollout. The rollout decisions themselves are tested in runtime_multi_test.go.

package unit

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"quantumtrustkg/controller/internal/controllers"
)

// TestKubernetesStatusWriterUpdatesMigrationPlanStatus checks phase and
// completedEdges after a write.
func TestKubernetesStatusWriterUpdatesMigrationPlanStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	template := &unstructured.Unstructured{}
	template.SetAPIVersion("quantumtrustkg.io/v1alpha1")
	template.SetKind("MigrationPlan")

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion("quantumtrustkg.io/v1alpha1")
	existing.SetKind("MigrationPlan")
	existing.SetNamespace("finance")
	existing.SetName("auth-staged")
	existing.Object["spec"] = map[string]any{
		"strategy": "staged",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(template).
		WithObjects(existing).
		Build()

	writer := controllers.NewKubernetesStatusWriter(client)
	plan := controllers.MigrationPlanSpec{
		Name:      "auth-staged",
		Namespace: "finance",
		Strategy:  "staged",
	}
	progress := controllers.MigrationProgress{
		CompletedEdges: map[string]bool{"frontendToAuth": true},
		LastPhase:      "StagedWaiting",
		LastBatchSize:  1,
	}

	if err := writer.WriteMigrationStatus(context.Background(), plan, progress); err != nil {
		t.Fatalf("expected migration status writeback to succeed, got %v", err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("quantumtrustkg.io/v1alpha1")
	obj.SetKind("MigrationPlan")
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Namespace: "finance", Name: "auth-staged"}, obj); err != nil {
		t.Fatalf("expected migration plan to exist, got %v", err)
	}

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase != "StagedWaiting" {
		t.Fatalf("expected staged waiting phase, got %s", phase)
	}
	completed, _, _ := unstructured.NestedInt64(obj.Object, "status", "completedEdges")
	if completed != 1 {
		t.Fatalf("expected completedEdges=1, got %d", completed)
	}
}
