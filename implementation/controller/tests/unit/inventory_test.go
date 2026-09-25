// Tests of the cluster inventory path (Sect. 3, implementation).
//
// In a live cluster the controller builds its edges from Services annotated with
// quantumtrustkg.io/calls and joins them with the CryptoCapabilityProfile records.
// These tests use a fake client and check three things. The inventory yields one
// edge per annotated call. A capability record can select a Service by any label
// and not only by its name. The runtime evaluates a whole namespace through the
// inventory and, when a graph is reachable, mirrors the edges into it and reads the
// candidate and policy queries back. The graph is a stub that counts requests, which means
// the tests check the wiring and not the SPARQL results.

package unit

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"quantumtrustkg/controller/internal/controllers"
	"quantumtrustkg/controller/internal/mesh"
	"quantumtrustkg/controller/pkg/types"
)

// TestClusterInventoryBuildsEdgesFromServicesAndCapabilities checks that two
// annotated Services yield two edges.
func TestClusterInventoryBuildsEdgesFromServicesAndCapabilities(t *testing.T) {
	scheme := apiruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	frontend := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "frontend",
			Annotations: map[string]string{
				"quantumtrustkg.io/calls":          "auth",
				"quantumtrustkg.io/trust-boundary": "internal",
			},
		},
	}
	auth := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "auth",
			Annotations: map[string]string{
				"quantumtrustkg.io/calls":          "payment",
				"quantumtrustkg.io/trust-boundary": "regulated",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(frontend, auth).
		Build()

	resources := controllers.NewResourceState()
	now := time.Now().UTC()
	resources.UpsertCapability(types.CapabilityProfile{
		Name:             "frontend-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "frontend"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	resources.UpsertCapability(types.CapabilityProfile{
		Name:             "auth-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "auth"},
		Profiles:         []string{"HybridProfile", "QuantumSafeProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	resources.UpsertCapability(types.CapabilityProfile{
		Name:             "payment-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "payment"},
		Profiles:         []string{"QuantumSafeProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})

	inventory := controllers.NewClusterInventorySource(client, resources)
	edges, err := inventory.BuildEdges(context.Background(), "finance")
	if err != nil {
		t.Fatalf("expected inventory build to succeed, got %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
}

// TestClusterInventoryMatchesCapabilitiesByServiceLabels checks that capability
// selectors on team and tier labels select the right Services and yield the
// hybrid candidate.
func TestClusterInventoryMatchesCapabilitiesByServiceLabels(t *testing.T) {
	scheme := apiruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	frontend := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "frontend",
			Labels: map[string]string{
				"team": "payments",
				"tier": "edge",
			},
			Annotations: map[string]string{
				"quantumtrustkg.io/calls": "auth",
			},
		},
	}
	auth := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "auth",
			Labels: map[string]string{
				"team": "payments",
				"tier": "core",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(frontend, auth).
		Build()

	resources := controllers.NewResourceState()
	now := time.Now().UTC()
	resources.UpsertCapability(types.CapabilityProfile{
		Name:             "edge-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"team": "payments", "tier": "edge"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	resources.UpsertCapability(types.CapabilityProfile{
		Name:             "core-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"team": "payments", "tier": "core"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})

	inventory := controllers.NewClusterInventorySource(client, resources)
	edges, err := inventory.BuildEdges(context.Background(), "finance")
	if err != nil {
		t.Fatalf("expected inventory build to succeed, got %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if len(edges[0].CandidateProfiles) != 1 || edges[0].CandidateProfiles[0].Name != "HybridProfile" {
		t.Fatalf("expected label-selected hybrid candidate profile, got %#v", edges[0].CandidateProfiles)
	}
}

// TestRuntimeReconcileClusterNamespaceUsesInventorySource checks the live path
// end to end without a graph. The edge is decided and promoted with HybridProfile.
func TestRuntimeReconcileClusterNamespaceUsesInventorySource(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	scheme := apiruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	frontend := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "frontend",
			Annotations: map[string]string{
				"quantumtrustkg.io/calls":          "auth",
				"quantumtrustkg.io/trust-boundary": "internal",
			},
		},
	}
	auth := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "auth",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(frontend, auth).Build()

	now := time.Now().UTC()
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "frontend-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "frontend"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "auth-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "auth"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Inventory = controllers.NewClusterInventorySource(client, runtime.Resources)

	edges, err := runtime.ReconcileClusterNamespace(context.Background(), "finance")
	if err != nil {
		t.Fatalf("expected cluster namespace reconcile to succeed, got %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected one reconciled edge, got %d", len(edges))
	}
	if edges[0].DesiredProfile != "HybridProfile" {
		t.Fatalf("expected hybrid profile from live inventory path, got %s", edges[0].DesiredProfile)
	}
}

// TestRuntimeReconcileClusterNamespaceSyncsAndReadsGraph checks that a reachable
// graph receives a snapshot update and answers the candidate and policy queries.
func TestRuntimeReconcileClusterNamespaceSyncsAndReadsGraph(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://graph.example/dataset", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	scheme := apiruntime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	frontend := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "frontend",
			Annotations: map[string]string{
				"quantumtrustkg.io/calls":          "auth",
				"quantumtrustkg.io/trust-boundary": "internal",
			},
		},
	}
	auth := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "finance",
			Name:      "auth",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(frontend, auth).Build()
	runtime.Inventory = controllers.NewClusterInventorySource(client, runtime.Resources)

	now := time.Now().UTC()
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "frontend-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "frontend"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "auth-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "auth"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       now,
		FreshnessWindow:  5 * time.Minute,
	})

	var syncCalls, queryCalls int
	runtime.Graph.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/dataset":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil
			case r.Method == http.MethodPost && r.URL.Path == "/dataset/update":
				syncCalls++
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			case r.Method == http.MethodPost && r.URL.Path == "/dataset/query":
				queryCalls++
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read query body: %v", err)
				}
				payload := string(body)
				switch {
				case strings.Contains(payload, "candidateProfile"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"results":{"bindings":[{"name":{"type":"literal","value":"HybridProfile"},"securityCategory":{"type":"literal","value":"hybrid"},"overheadScore":{"type":"literal","value":"1.5"}}]}}`)),
						Header:     make(http.Header),
					}, nil
				case strings.Contains(payload, "policyName"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"results":{"bindings":[]}}`)),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected graph query payload: %s", payload)
				}
			}
			t.Fatalf("unexpected graph request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}),
	}

	edges, err := runtime.ReconcileClusterNamespace(context.Background(), "finance")
	if err != nil {
		t.Fatalf("expected graph-backed cluster reconcile to succeed, got %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected one reconciled edge, got %d", len(edges))
	}
	if syncCalls == 0 {
		t.Fatal("expected cluster snapshot sync to graph")
	}
	if queryCalls == 0 {
		t.Fatal("expected graph-backed candidate/policy queries during reconcile")
	}
}
