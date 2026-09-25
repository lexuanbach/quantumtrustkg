// Test of the AssignmentStatus writer (Sect. 3, auditable status).
//
// TestKubernetesStatusWriterCreatesAssignmentStatusObject writes a promoted status
// through KubernetesStatusWriter into a fake API server. It checks that the object is
// created, that the spec carries the edge endpoints and that status.phase equals the
// assignment state. The record is the controller-owned AssignmentStatus of Sect. 3,
// which stores the desired and active profile, the state and the reason. The fake
// client stands in for the API server, which means the test does not exercise RBAC.

package unit

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"quantumtrustkg/controller/internal/controllers"
	"quantumtrustkg/controller/pkg/types"
)

// TestKubernetesStatusWriterCreatesAssignmentStatusObject creates a status on
// first write and reads it back.
func TestKubernetesStatusWriterCreatesAssignmentStatusObject(t *testing.T) {
	scheme := runtime.NewScheme()
	template := &unstructured.Unstructured{}
	template.SetAPIVersion("quantumtrustkg.io/v1alpha1")
	template.SetKind("AssignmentStatus")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(template).
		Build()

	writer := controllers.NewKubernetesStatusWriter(client)
	status := types.AssignmentStatus{
		ID:                 "authToPayment",
		Namespace:          "finance",
		SourceService:      "Auth",
		DestinationService: "Payment",
		DesiredProfile:     "QuantumSafeProfile",
		ActiveProfile:      "QuantumSafeProfile",
		State:              types.AssignmentPromoted,
		Reason:             "assignment-applied",
		LastEvaluatedAt:    time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC),
	}

	if err := writer.WriteAssignmentStatus(context.Background(), status); err != nil {
		t.Fatalf("expected writeback to succeed, got %v", err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("quantumtrustkg.io/v1alpha1")
	obj.SetKind("AssignmentStatus")
	if err := client.Get(context.Background(), ctrlclient.ObjectKey{Namespace: status.Namespace, Name: status.ID}, obj); err != nil {
		t.Fatalf("expected assignment status object to exist, got %v", err)
	}

	source, _, _ := unstructured.NestedString(obj.Object, "spec", "sourceService")
	if source != "Auth" {
		t.Fatalf("expected sourceService Auth, got %s", source)
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase != string(types.AssignmentPromoted) {
		t.Fatalf("expected phase %s, got %s", types.AssignmentPromoted, phase)
	}
}
