// Multi-edge runtime tests (Sect. 3 and Sect. 4, G1 to G4 end to end).
//
// These tests drive AssignmentRuntime over the finance fixture and check whole
// reconciliations, including the paths the paper's evaluation relies on.
//
//   all edges          every edge of the fixture is promoted and stored
//   stale and          the stale and contradictory fixtures block with the
//   conflicting facts  reasons stale-facts and conflicting-facts (G4)
//   graph              live SPARQL queries are used when the graph answers.
//                      During an outage with FailClosedOnGraphUnavailable the
//                      edges are blocked, and an edge with a committed profile
//                      keeps it (G4)
//   overlays           watched policies, capability records and label selectors
//                      narrow the candidates and raise the required class (G2)
//   policy conflict    a strict quantum-safe policy with hybrid-only endpoints
//                      blocks and does not fall back
//   staged rollout     a staged MigrationPlan defers edges, advances one batch per
//                      reconciliation after the previous one is confirmed healthy
//                      and freezes when a promoted edge turns unhealthy
//
// The staged rollout tests correspond to the rollout numbers of RQ3 only in
// mechanism. The rollout durations and failure rates of Sect. 4.3 come from
// controller logs of the in-cluster runs and not from these tests.

package unit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quantumtrustkg/controller/internal/controllers"
	"quantumtrustkg/controller/internal/mesh"
	"quantumtrustkg/controller/pkg/types"
)

// TestReconcileFixtureEdgesProducesAssignmentsForAllEdges checks the three
// finance edges end Promoted with desired and active profiles and are stored.
func TestReconcileFixtureEdgesProducesAssignmentsForAllEdges(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)

	edges, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected fixture reconciliation to succeed, got %v", err)
	}

	if len(edges) != 3 {
		t.Fatalf("expected 3 reconciled edges, got %d", len(edges))
	}

	for _, edge := range edges {
		if edge.DesiredProfile == "" {
			t.Fatalf("expected desired profile for edge %s", edge.ID)
		}
		if edge.ActiveProfile == "" {
			t.Fatalf("expected active profile for edge %s", edge.ID)
		}
		if edge.State != types.AssignmentPromoted {
			t.Fatalf("expected promoted state for edge %s, got %s", edge.ID, edge.State)
		}
		if _, ok := runtime.Store.Get(edge.ID); !ok {
			t.Fatalf("expected edge %s to be stored", edge.ID)
		}
		status, ok := runtime.StatusStore.Get(edge.ID)
		if !ok {
			t.Fatalf("expected status for edge %s to be stored", edge.ID)
		}
		if status.State != types.AssignmentPromoted {
			t.Fatalf("expected promoted status for edge %s, got %s", edge.ID, status.State)
		}
	}
}

// TestReconcileFixtureEdgesFailsClosedOnStaleFacts checks the stale-facts block.
func TestReconcileFixtureEdgesFailsClosedOnStaleFacts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)

	_, err := runtime.ReconcileFixtureEdges(context.Background(), "stale-finance-scenario.ttl")
	if err == nil {
		t.Fatal("expected stale fixture reconciliation to fail")
	}
	status, ok := runtime.StatusStore.Get("authToPayment")
	if !ok {
		t.Fatal("expected blocked status to be recorded")
	}
	if status.State != types.AssignmentBlocked || status.Reason != "stale-facts" {
		t.Fatalf("expected stale blocked status, got state=%s reason=%s", status.State, status.Reason)
	}
}

// TestReconcileFixtureEdgesFailsClosedOnConflictingFacts checks the
// conflicting-facts block.
func TestReconcileFixtureEdgesFailsClosedOnConflictingFacts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)

	_, err := runtime.ReconcileFixtureEdges(context.Background(), "conflicting-finance-scenario.ttl")
	if err == nil {
		t.Fatal("expected conflicting fixture reconciliation to fail")
	}
	status, ok := runtime.StatusStore.Get("authToPayment")
	if !ok {
		t.Fatal("expected blocked status to be recorded")
	}
	if status.State != types.AssignmentBlocked || status.Reason != "conflicting-facts" {
		t.Fatalf("expected conflicting blocked status, got state=%s reason=%s", status.State, status.Reason)
	}
}

// TestReconcileFixtureEdgesUsesLiveGraphQueriesWhenAvailable checks that a
// reachable graph is queried for candidates and policies and receives one update
// per edge.
func TestReconcileFixtureEdgesUsesLiveGraphQueriesWhenAvailable(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://graph.example/dataset", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	var queryCalls, updateCalls int
	runtime.Graph.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/dataset":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
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
					response := `{"results":{"bindings":[{"name":{"type":"literal","value":"HybridProfile"},"securityCategory":{"type":"literal","value":"hybrid"},"overheadScore":{"type":"literal","value":"2.0"}}]}}`
					if strings.Contains(payload, "authToPayment") || strings.Contains(payload, "paymentToLedger") {
						response = `{"results":{"bindings":[{"name":{"type":"literal","value":"QuantumSafeProfile"},"securityCategory":{"type":"literal","value":"quantum_safe"},"overheadScore":{"type":"literal","value":"2.5"}}]}}`
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(response)),
						Header:     make(http.Header),
					}, nil
				case strings.Contains(payload, "policyName"):
					response := `{"results":{"bindings":[{"name":{"type":"literal","value":"LivePolicy"},"requiredLevel":{"type":"literal","value":"hybrid"}}]}}`
					if strings.Contains(payload, "authToPayment") || strings.Contains(payload, "paymentToLedger") {
						response = `{"results":{"bindings":[{"name":{"type":"literal","value":"LivePolicy"},"requiredLevel":{"type":"literal","value":"quantum_safe"}}]}}`
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(response)),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected query payload: %s", payload)
				}
			case r.Method == http.MethodPost && r.URL.Path == "/dataset/update":
				updateCalls++
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			}
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}),
	}

	edges, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected reconciliation to succeed with live graph, got %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(edges))
	}
	if queryCalls == 0 {
		t.Fatal("expected live graph queries to be used")
	}
	if updateCalls != 3 {
		t.Fatalf("expected one graph update per edge, got %d", updateCalls)
	}
}

// TestReconcileFixtureEdgesFailsClosedOnGraphOutageWhenRequired checks that a
// graph outage blocks every edge with reason graph-unavailable and selects no
// profile (Thm. 2).
func TestReconcileFixtureEdgesFailsClosedOnGraphOutageWhenRequired(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://graph.example/dataset", root)
	runtime.FailClosedOnGraphUnavailable = true
	runtime.Graph.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("graph outage")
		}),
	}

	edges, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected outage to produce blocked-safe statuses without selecting profiles, got %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("expected 3 blocked edges, got %d", len(edges))
	}
	for _, edge := range edges {
		if edge.State != types.AssignmentBlocked || edge.Reason != "graph-unavailable" {
			t.Fatalf("expected graph-unavailable block for %s, got state=%s reason=%s", edge.ID, edge.State, edge.Reason)
		}
		if status, ok := runtime.StatusStore.Get(edge.ID); !ok || status.State != types.AssignmentBlocked {
			t.Fatalf("expected blocked status during outage for %s", edge.ID)
		}
	}
}

// TestReconcileFixtureEdgesPreservesActiveAssignmentDuringGraphOutage checks that
// an edge with a committed profile keeps it during an outage and reports
// graph-unavailable-preserved-active.
func TestReconcileFixtureEdgesPreservesActiveAssignmentDuringGraphOutage(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://graph.example/dataset", root)
	runtime.FailClosedOnGraphUnavailable = true
	runtime.Store.Upsert(types.CommunicationEdge{
		ID:             "authToPayment",
		Namespace:      "finance",
		SourceService:  "Auth",
		ActiveProfile:  "QuantumSafeProfile",
		DesiredProfile: "QuantumSafeProfile",
		State:          types.AssignmentPromoted,
	})
	runtime.Graph.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("graph outage")
		}),
	}

	edge, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Auth", "Payment")
	if err != nil {
		t.Fatalf("expected outage to preserve active assignment without error, got %v", err)
	}
	if edge.State != types.AssignmentBlocked || edge.Reason != "graph-unavailable-preserved-active" {
		t.Fatalf("expected preserved-active block, got state=%s reason=%s", edge.State, edge.Reason)
	}
	if edge.ActiveProfile != "QuantumSafeProfile" || edge.DesiredProfile != "QuantumSafeProfile" {
		t.Fatalf("expected active assignment to be preserved, got active=%s desired=%s", edge.ActiveProfile, edge.DesiredProfile)
	}
}

// TestReconcileFixtureEdgesAppliesWatchedResourceOverlays checks that a watched
// policy raises Req(e) to quantum-safe and that capability records narrow the
// candidates to QuantumSafeProfile.
func TestReconcileFixtureEdgesAppliesWatchedResourceOverlays(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	runtime.Resources.UpsertPolicy(types.Policy{
		Name:                     "frontend-upgrade",
		Namespace:                "finance",
		Selector:                 map[string]string{"app": "Frontend"},
		RequiredSecurityCategory: types.SecurityQuantumSafe,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "frontend-quantum-safe",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "Frontend"},
		Profiles:         []string{"QuantumSafeProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "auth-quantum-safe",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "Auth"},
		Profiles:         []string{"QuantumSafeProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})

	edge, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth")
	if err != nil {
		t.Fatalf("expected targeted reconciliation to succeed, got %v", err)
	}
	if edge.RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected watched policy to raise required level, got %s", edge.RequiredSecurityLevel)
	}
	if edge.DesiredProfile != "QuantumSafeProfile" {
		t.Fatalf("expected watched capabilities to narrow profile selection, got %s", edge.DesiredProfile)
	}
}

// TestReconcileFixtureEdgesBlocksPolicyConflictWithoutHybridFallback checks that
// hybrid-only endpoints under a strict quantum-safe policy without allowHybrid
// end blocked with no-admissible-profile (G2).
func TestReconcileFixtureEdgesBlocksPolicyConflictWithoutHybridFallback(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	runtime.Resources.UpsertPolicy(types.Policy{
		Name:                     "strict-quantum-safe",
		Namespace:                "finance",
		Selector:                 map[string]string{"app": "Frontend"},
		RequiredSecurityCategory: types.SecurityQuantumSafe,
		AllowHybrid:              false,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "frontend-hybrid-only",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "Frontend"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "auth-hybrid-only",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"app": "Auth"},
		Profiles:         []string{"HybridProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})

	_, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth")
	if err == nil {
		t.Fatal("expected strict policy/capability conflict to block selection")
	}
	status, ok := runtime.StatusStore.Get("frontendToAuth")
	if !ok {
		t.Fatal("expected blocked status for frontendToAuth")
	}
	if status.State != types.AssignmentBlocked || status.Reason != "no-admissible-profile" {
		t.Fatalf("expected no-admissible-profile block, got state=%s reason=%s", status.State, status.Reason)
	}
}

// TestReconcileFixtureEdgesAppliesLabelScopedOverlays repeats the overlay test
// with selectors on team and tier labels instead of service names.
func TestReconcileFixtureEdgesAppliesLabelScopedOverlays(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	runtime.Resources.UpsertServiceLabels("finance", "Frontend", map[string]string{
		"team": "payments",
		"tier": "edge",
	})
	runtime.Resources.UpsertServiceLabels("finance", "Auth", map[string]string{
		"team": "payments",
		"tier": "core",
	})

	runtime.Resources.UpsertPolicy(types.Policy{
		Name:                     "payments-edge-upgrade",
		Namespace:                "finance",
		Selector:                 map[string]string{"team": "payments", "tier": "edge"},
		RequiredSecurityCategory: types.SecurityQuantumSafe,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "payments-edge-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"team": "payments", "tier": "edge"},
		Profiles:         []string{"QuantumSafeProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name:             "payments-core-capability",
		Namespace:        "finance",
		WorkloadSelector: map[string]string{"team": "payments", "tier": "core"},
		Profiles:         []string{"QuantumSafeProfile"},
		ObservedAt:       time.Now().UTC(),
		FreshnessWindow:  5 * time.Minute,
	})

	edge, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth")
	if err != nil {
		t.Fatalf("expected targeted reconciliation to succeed, got %v", err)
	}
	if edge.RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected label-scoped policy to raise required level, got %s", edge.RequiredSecurityLevel)
	}
	if edge.DesiredProfile != "QuantumSafeProfile" {
		t.Fatalf("expected label-scoped capabilities to narrow profile selection, got %s", edge.DesiredProfile)
	}
}

// TestReconcileFixtureEdgesDefersEdgesOutsideStagedRolloutWindow checks that a
// staged plan with one unavailable edge defers a targeted edge as Pending with
// reason awaiting-rollout-window (rollout feasibility, gate (v)).
func TestReconcileFixtureEdgesDefersEdgesOutsideStagedRolloutWindow(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	runtime.Resources.UpsertMigration(controllers.MigrationPlanSpec{
		Name:                "auth-staged",
		Namespace:           "finance",
		TargetSelector:      map[string]string{"app": "Auth"},
		Strategy:            "staged",
		MaxUnavailableEdges: 1,
		RollbackOnError:     true,
	})

	edges, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected staged reconciliation to succeed, got %v", err)
	}

	var deferred bool
	for _, edge := range edges {
		if edge.SourceService == "Auth" || edge.DestinationService == "Auth" {
			if edge.State == types.AssignmentPending && edge.Reason == "awaiting-rollout-window" {
				deferred = true
			}
		}
	}
	if !deferred {
		t.Fatal("expected at least one targeted edge to be deferred by staged rollout")
	}
}

// TestReconcileFixtureEdgesAdvancesStagedRolloutAcrossReconciliations checks that
// the first pass promotes one targeted edge, which then waits for health
// confirmation, and that the second pass completes it and promotes the next.
func TestReconcileFixtureEdgesAdvancesStagedRolloutAcrossReconciliations(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	runtime.Resources.UpsertMigration(controllers.MigrationPlanSpec{
		Name:                "auth-staged",
		Namespace:           "finance",
		TargetSelector:      map[string]string{"app": "Auth"},
		Strategy:            "staged",
		MaxUnavailableEdges: 1,
		RollbackOnError:     true,
	})

	first, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected first staged reconciliation to succeed, got %v", err)
	}
	progressAfterFirst := runtime.Resources.MigrationProgress(controllers.MigrationPlanSpec{
		Name:      "auth-staged",
		Namespace: "finance",
	})
	second, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected second staged reconciliation to succeed, got %v", err)
	}
	progressAfterSecond := runtime.Resources.MigrationProgress(controllers.MigrationPlanSpec{
		Name:      "auth-staged",
		Namespace: "finance",
	})

	firstPromoted := 0
	secondPromoted := 0
	for _, edge := range first {
		if (edge.SourceService == "Auth" || edge.DestinationService == "Auth") && edge.State == types.AssignmentPromoted {
			firstPromoted++
		}
	}
	for _, edge := range second {
		if (edge.SourceService == "Auth" || edge.DestinationService == "Auth") && edge.State == types.AssignmentPromoted {
			secondPromoted++
		}
	}

	if firstPromoted != 1 {
		t.Fatalf("expected exactly one targeted edge in first rollout batch, got %d", firstPromoted)
	}
	if len(progressAfterFirst.CompletedEdges) != 0 || len(progressAfterFirst.PendingHealthEdges) != 1 {
		t.Fatalf("expected first batch to wait on health confirmation, got completed=%d pending=%d", len(progressAfterFirst.CompletedEdges), len(progressAfterFirst.PendingHealthEdges))
	}
	if secondPromoted != 2 {
		t.Fatalf("expected staged rollout to advance on second reconcile, got %d promoted edges", secondPromoted)
	}
	if len(progressAfterSecond.CompletedEdges) != 1 || len(progressAfterSecond.PendingHealthEdges) != 1 {
		t.Fatalf("expected one healthy completed edge and one new pending edge, got completed=%d pending=%d", len(progressAfterSecond.CompletedEdges), len(progressAfterSecond.PendingHealthEdges))
	}
}

// TestReconcileFixtureEdgesDoesNotAdvanceWhenPreviousBatchTurnsUnhealthy checks
// that an unhealthy marker on a promoted edge puts the plan in the Rollback phase
// and freezes the rollout.
func TestReconcileFixtureEdgesDoesNotAdvanceWhenPreviousBatchTurnsUnhealthy(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh

	plan := controllers.MigrationPlanSpec{
		Name:                "auth-staged",
		Namespace:           "finance",
		TargetSelector:      map[string]string{"app": "Auth"},
		Strategy:            "staged",
		MaxUnavailableEdges: 1,
		RollbackOnError:     true,
	}
	runtime.Resources.UpsertMigration(plan)

	first, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected first staged reconciliation to succeed, got %v", err)
	}

	var firstPromotedEdgeID string
	for _, edge := range first {
		if (edge.SourceService == "Auth" || edge.DestinationService == "Auth") && edge.State == types.AssignmentPromoted {
			firstPromotedEdgeID = edge.ID
			if err := runtime.Mesh.MarkEdgeUnhealthy(edge.Namespace, edge.ID, "health-check-failed"); err != nil {
				t.Fatalf("expected unhealthy marker write to succeed, got %v", err)
			}
			break
		}
	}
	if firstPromotedEdgeID == "" {
		t.Fatal("expected a promoted edge in first batch")
	}

	second, err := runtime.ReconcileFixtureEdges(context.Background(), "finance-scenario.ttl")
	if err != nil {
		t.Fatalf("expected second staged reconciliation to complete, got %v", err)
	}
	progress := runtime.Resources.MigrationProgress(plan)
	if progress.LastPhase != "Rollback" {
		t.Fatalf("expected rollback phase after unhealthy batch, got %s", progress.LastPhase)
	}

	secondPromoted := 0
	for _, edge := range second {
		if (edge.SourceService == "Auth" || edge.DestinationService == "Auth") && edge.State == types.AssignmentPromoted {
			secondPromoted++
		}
	}
	if secondPromoted != 0 {
		t.Fatalf("expected rollout freeze after unhealthy batch, got %d promoted edges", secondPromoted)
	}
}
