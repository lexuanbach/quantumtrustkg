// Tests of the mesh layer (Sect. 3, Istio integration).
//
// The controller turns an assignment into a PeerAuthentication and a
// DestinationRule (internal/mesh), stores them, and observes readiness before it
// records the profile as active.
//
//   synthesis    the two objects exist, the profile is an annotation and the
//                destination rule targets the destination host
//   validation   duplicate resources are rejected before anything is written
//   filesystem   manifests are written under <dir>/<namespace>, and the .applied
//                and .unhealthy markers drive the fixture-mode health reading
//   cluster      ClusterHealthObserver reports Healthy when both objects exist
//                and both workloads are Ready, and Unhealthy when the destination
//                pod is not Ready (the readiness gate of Thm. 2)
//
// A fake Kubernetes client replaces the API server. No Istio installation is
// involved, which means the tests check the objects the controller would apply and not the
// behaviour of the mesh.

package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"quantumtrustkg/controller/internal/mesh"
	"quantumtrustkg/controller/pkg/types"
)

// TestSynthesizeAssignmentBuildsIstioResources checks the two resource kinds,
// the profile annotation and the destination host.
func TestSynthesizeAssignmentBuildsIstioResources(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "authToPayment",
		Namespace:             "finance",
		SourceService:         "auth",
		DestinationService:    "payment",
		TrustBoundary:         "regulated",
		DesiredProfile:        "QuantumSafeProfile",
		RequiredSecurityLevel: types.SecurityQuantumSafe,
	}

	resources := mesh.SynthesizeAssignment(edge)
	if len(resources) != 2 {
		t.Fatalf("expected 2 synthesized resources, got %d", len(resources))
	}

	if resources[0].Kind != "PeerAuthentication" {
		t.Fatalf("expected first resource to be PeerAuthentication, got %s", resources[0].Kind)
	}
	if !strings.Contains(resources[0].Manifest, "quantumtrustkg.io/profile: QuantumSafeProfile") {
		t.Fatal("expected peer authentication manifest to include profile annotation")
	}
	if resources[1].Kind != "DestinationRule" {
		t.Fatalf("expected second resource to be DestinationRule, got %s", resources[1].Kind)
	}
	if !strings.Contains(resources[1].Manifest, "host: payment.finance.svc.cluster.local") {
		t.Fatal("expected destination rule host to target destination service")
	}
}

// TestApplyResourcesRejectsDuplicates checks that a batch with a repeated resource
// identifier fails validation.
func TestApplyResourcesRejectsDuplicates(t *testing.T) {
	resource := mesh.Resource{
		Kind:      "PeerAuthentication",
		Namespace: "finance",
		Name:      "auth-topayment-mtls",
		Manifest:  "kind: PeerAuthentication",
	}

	if err := mesh.ApplyResources([]mesh.Resource{resource, resource}); err == nil {
		t.Fatal("expected duplicate resource validation to fail")
	}
}

// TestApplierPersistsResourcesToGeneratedDirectory checks the file name and the
// content of a stored manifest in filesystem mode.
func TestApplierPersistsResourcesToGeneratedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	applier := mesh.NewApplier(tmpDir)
	applier.Mode = mesh.ApplyModeFilesystem

	resources := []mesh.Resource{
		{
			Kind:      "PeerAuthentication",
			Namespace: "finance",
			Name:      "auth-topayment-mtls",
			Manifest:  "kind: PeerAuthentication\nmetadata:\n  name: auth-topayment-mtls\n",
		},
	}

	if err := applier.ApplyResources(context.Background(), resources); err != nil {
		t.Fatalf("expected filesystem apply to succeed, got %v", err)
	}

	path := filepath.Join(tmpDir, "finance", "peerauthentication-auth-topayment-mtls.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected manifest file to be written, got %v", err)
	}
	if !strings.Contains(string(data), "kind: PeerAuthentication") {
		t.Fatal("expected persisted manifest contents")
	}
}

// TestApplierObservesEdgeHealthFromMarkers checks that an applied edge reads as
// healthy and that an unhealthy marker overrides it.
func TestApplierObservesEdgeHealthFromMarkers(t *testing.T) {
	tmpDir := t.TempDir()
	applier := mesh.NewApplier(tmpDir)
	applier.Mode = mesh.ApplyModeFilesystem

	resources := []mesh.Resource{
		{
			Kind:      "PeerAuthentication",
			Namespace: "finance",
			Name:      "auth-topayment-mtls",
			EdgeID:    "authToPayment",
			Manifest:  "kind: PeerAuthentication\nmetadata:\n  name: auth-topayment-mtls\n",
		},
	}
	if err := applier.ApplyResources(context.Background(), resources); err != nil {
		t.Fatalf("expected apply to succeed, got %v", err)
	}
	if applier.ObserveEdgeHealth(types.CommunicationEdge{Namespace: "finance", ID: "authToPayment"}) != mesh.EdgeHealthHealthy {
		t.Fatal("expected applied edge to be observed as healthy")
	}
	if err := applier.MarkEdgeUnhealthy("finance", "authToPayment", "probe failed"); err != nil {
		t.Fatalf("expected unhealthy marker write to succeed, got %v", err)
	}
	if applier.ObserveEdgeHealth(types.CommunicationEdge{Namespace: "finance", ID: "authToPayment"}) != mesh.EdgeHealthUnhealthy {
		t.Fatal("expected unhealthy marker to override health observation")
	}
}

// TestClusterHealthObserverChecksIstioResourcesAndPods checks the cluster
// observer against a fake client that holds both mesh objects and one Ready pod
// per endpoint. Setting the destination pod to not Ready changes the reading to
// Unhealthy.
func TestClusterHealthObserverChecksIstioResourcesAndPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	peerAuth := &unstructured.Unstructured{}
	peerAuth.SetAPIVersion("security.istio.io/v1beta1")
	peerAuth.SetKind("PeerAuthentication")
	peerAuth.SetNamespace("finance")
	peerAuth.SetName("authToPayment-mtls")

	destRule := &unstructured.Unstructured{}
	destRule.SetAPIVersion("networking.istio.io/v1beta1")
	destRule.SetKind("DestinationRule")
	destRule.SetNamespace("finance")
	destRule.SetName("authToPayment-tls")

	sourcePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "auth-0",
			Labels:    map[string]string{"app": "auth"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
	destPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "payment-0",
			Labels:    map[string]string{"app": "payment"},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(peerAuth, destRule, sourcePod, destPod).
		Build()

	observer := mesh.NewClusterHealthObserver(client)
	edge := types.CommunicationEdge{
		ID:                 "authToPayment",
		Namespace:          "finance",
		SourceService:      "auth",
		DestinationService: "payment",
	}
	if observer.ObserveEdgeHealth(edge) != mesh.EdgeHealthHealthy {
		t.Fatal("expected cluster observer to report healthy edge")
	}

	destPod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	if err := client.Status().Update(context.Background(), destPod); err != nil {
		t.Fatalf("expected pod status update to succeed, got %v", err)
	}
	if observer.ObserveEdgeHealth(edge) != mesh.EdgeHealthUnhealthy {
		t.Fatal("expected cluster observer to report unhealthy edge when destination pod is not ready")
	}
}
