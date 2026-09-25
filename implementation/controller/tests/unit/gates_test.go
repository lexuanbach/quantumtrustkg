// Tests of the individual QTPO gates and the promotion rule (Sect. 3,
// Alg. 1, Thm. 1 and Thm. 2).
//
// Each group below targets one gate or one clause of the promotion rule and checks
// both the outcome and the reason string that the controller records.
//
//   C_e gate            CompatibleProfiles blocks on a missing, stale or disjoint
//                       capability record (G4). Only the intersection of the two
//                       endpoints' sets survives.
//   Runtime blocking    The runtime records a blocked status with a named reason
//                       and does not record a new profile (G4).
//   Readiness gate      A profile becomes active only when both workloads are
//                       Ready. An unhealthy reading fails the edge and keeps the
//                       previous profile. This is the staged promotion of Sect. 3.
//   Threat gates        Deprecated families, insufficient proof status and active
//                       advisories remove a candidate (gates (ii) and (iii)). A
//                       hybrid fallback needs a robust combiner.
//   Score configuration The default weights are the operating point of Sect. 3.
//                       The weighted score and the hysteresis margin change which
//                       admissible profile wins, and they do not change what is
//                       admissible (Prop. 1).
//   Pre-commit checker  CheckPromotion mirrors the gate order and returns the
//                       first failing gate as its reason.

package unit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"quantumtrustkg/controller/internal/controllers"
	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/internal/mesh"
	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// C_e gate

// knownCaps builds fresh endpoint capabilities that advertise the given profiles.
func knownCaps(profiles ...string) types.EndpointCapabilities {
	return types.EndpointCapabilities{Known: true, Profiles: profiles}
}

// capabilityEdge builds an edge with a hybrid requirement, a hybrid and a
// quantum-safe candidate and the given endpoint capabilities.
func capabilityEdge(src, dst types.EndpointCapabilities) types.CommunicationEdge {
	return types.CommunicationEdge{
		ID:                      "cap-edge",
		SourceService:           "Src",
		DestinationService:      "Dst",
		RequiredSecurityLevel:   types.SecurityHybrid,
		SourceCapabilities:      src,
		DestinationCapabilities: dst,
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "HybridProfile", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.2},
			{Name: "QuantumSafeProfile", SecurityCategory: types.SecurityQuantumSafe, OverheadScore: 1.5},
		},
	}
}

// TestCompatibleProfilesFailsClosed checks the failure reason and the size of
// C_e for a missing or stale record on either side, disjoint sets, an empty set
// and a common profile.
func TestCompatibleProfilesFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		src    types.EndpointCapabilities
		dst    types.EndpointCapabilities
		reason string
		want   int
	}{
		{"missing source", types.EndpointCapabilities{}, knownCaps("HybridProfile"), "missing-capability:Src", 0},
		{"missing destination", knownCaps("HybridProfile"), types.EndpointCapabilities{}, "missing-capability:Dst", 0},
		{"stale source", types.EndpointCapabilities{Stale: true}, knownCaps("HybridProfile"), "stale-capability:Src", 0},
		{"stale destination", knownCaps("HybridProfile"), types.EndpointCapabilities{Stale: true}, "stale-capability:Dst", 0},
		{"disjoint non-empty sets", knownCaps("HybridProfile"), knownCaps("QuantumSafeProfile"), "no-common-profile", 0},
		{"empty advertised set", knownCaps(), knownCaps("HybridProfile"), "no-common-profile", 0},
		{"common profile", knownCaps("HybridProfile", "QuantumSafeProfile"), knownCaps("QuantumSafeProfile"), "", 1},
	}
	for _, tc := range cases {
		got, reason := orchestration.CompatibleProfiles(capabilityEdge(tc.src, tc.dst))
		if reason != tc.reason || len(got) != tc.want {
			t.Fatalf("%s: got %d candidates reason=%q, want %d reason=%q", tc.name, len(got), reason, tc.want, tc.reason)
		}
	}
}

// TestFixtureEndpointWithoutAdvertisedProfilesSupportsNothing checks that a
// fixture endpoint with no advertised profiles yields no candidates and the
// reason missing-capability. An endpoint that advertises nothing must not be
// read as supporting everything.
func TestFixtureEndpointWithoutAdvertisedProfilesSupportsNothing(t *testing.T) {
	ttl := []byte(`@prefix : <http://quantumtrustkg.io/ontology#> .
:A a :Service ;
  :supportsProfile :HybridProfile .
:B a :Service ;
  :subjectTo :DefaultInternalPolicy .
:HybridProfile a :Protocol ;
  :hasSecurityCategory "hybrid" ;
  :hasOverheadScore "1.2" .
:DefaultInternalPolicy a :Policy ;
  :requiresLevel "hybrid" .
:aToB a :Communication ;
  :sourceService :A ;
  :destinationService :B ;
  :crossesBoundary "internal" .
`)
	edges, err := graph.BuildEdgesFromTTL(ttl)
	if err != nil || len(edges) != 1 {
		t.Fatalf("build edges: %v (%d)", err, len(edges))
	}
	if len(edges[0].CandidateProfiles) != 0 {
		t.Fatalf("an endpoint without advertised profiles must yield no candidates, got %d", len(edges[0].CandidateProfiles))
	}
	if _, reason := orchestration.CompatibleProfiles(edges[0]); reason != "missing-capability:B" {
		t.Fatalf("expected missing-capability:B, got %q", reason)
	}
}

// newGateRuntime builds a runtime with a temporary filesystem applier that also
// serves as health observer. The graph endpoint is unreachable, which means the fixture
// path is used.
func newGateRuntime(t *testing.T) *controllers.AssignmentRuntime {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	runtime := controllers.NewAssignmentRuntime("http://example.invalid", root)
	runtime.Mesh = mesh.NewApplier(t.TempDir())
	runtime.Mesh.Mode = mesh.ApplyModeFilesystem
	runtime.HealthObserver = runtime.Mesh
	return runtime
}

// TestRuntimeBlocksDisjointEndpointCapabilities checks that endpoints with
// disjoint capability sets produce a blocked status with reason no-common-profile
// and no recorded profile.
func TestRuntimeBlocksDisjointEndpointCapabilities(t *testing.T) {
	runtime := newGateRuntime(t)
	now := time.Now().UTC()
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name: "frontend-hybrid", Namespace: "finance", WorkloadSelector: map[string]string{"app": "Frontend"},
		Profiles: []string{"HybridProfile"}, ObservedAt: now, FreshnessWindow: 5 * time.Minute,
	})
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name: "auth-qs", Namespace: "finance", WorkloadSelector: map[string]string{"app": "Auth"},
		Profiles: []string{"QuantumSafeProfile"}, ObservedAt: now, FreshnessWindow: 5 * time.Minute,
	})
	if _, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth"); err == nil {
		t.Fatal("expected disjoint endpoint capabilities to block the edge")
	}
	status, ok := runtime.StatusStore.Get("frontendToAuth")
	if !ok || status.State != types.AssignmentBlocked || status.Reason != "no-common-profile" {
		t.Fatalf("expected no-common-profile block, got %+v", status)
	}
	if status.ActiveProfile == "QuantumSafeProfile" || status.DesiredProfile != "" {
		t.Fatalf("a blocked edge must not record a new profile, got desired=%q active=%q", status.DesiredProfile, status.ActiveProfile)
	}
}

// TestRuntimeBlocksStaleEndpointCapability checks that a capability record
// observed an hour ago with a five-minute window blocks the edge as stale
// (gate (iv), G4).
func TestRuntimeBlocksStaleEndpointCapability(t *testing.T) {
	runtime := newGateRuntime(t)
	now := time.Now().UTC()
	runtime.Resources.UpsertCapability(types.CapabilityProfile{
		Name: "auth-old", Namespace: "finance", WorkloadSelector: map[string]string{"app": "Auth"},
		Profiles: []string{"HybridProfile", "QuantumSafeProfile"}, ObservedAt: now.Add(-time.Hour), FreshnessWindow: 5 * time.Minute,
	})
	if _, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth"); err == nil {
		t.Fatal("expected a stale endpoint capability record to block the edge")
	}
	status, _ := runtime.StatusStore.Get("frontendToAuth")
	if status.State != types.AssignmentBlocked || status.Reason != "stale-capability:Auth" {
		t.Fatalf("expected stale-capability:Auth block, got state=%s reason=%s", status.State, status.Reason)
	}
}

// Readiness gate

// scriptedHealth is a health observer that returns a scripted sequence of
// readings and then Unknown.

type scriptedHealth struct{ results []mesh.EdgeHealth }

// ObserveEdgeHealth returns the next scripted reading.
func (s *scriptedHealth) ObserveEdgeHealth(types.CommunicationEdge) mesh.EdgeHealth {
	if len(s.results) == 0 {
		return mesh.EdgeHealthUnknown
	}
	h := s.results[0]
	s.results = s.results[1:]
	return h
}

// TestRuntimeRecordsActiveProfileOnlyAfterWorkloadsReady checks the staged
// promotion. With an Unknown reading the edge stays Applying and the desired
// profile is set. After a Healthy reading the same edge is Promoted and the
// profile becomes active.
func TestRuntimeRecordsActiveProfileOnlyAfterWorkloadsReady(t *testing.T) {
	runtime := newGateRuntime(t)
	health := &scriptedHealth{results: []mesh.EdgeHealth{mesh.EdgeHealthUnknown, mesh.EdgeHealthHealthy}}
	runtime.HealthObserver = health

	first, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Auth", "Payment")
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	status, _ := runtime.StatusStore.Get("authToPayment")
	if first.State != types.AssignmentApplying || status.State != types.AssignmentApplying {
		t.Fatalf("before readiness the edge must be Applying, got edge=%s status=%s", first.State, status.State)
	}
	if status.DesiredProfile != "QuantumSafeProfile" {
		t.Fatalf("expected desired QuantumSafeProfile, got %q", status.DesiredProfile)
	}

	second, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Auth", "Payment")
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	status, _ = runtime.StatusStore.Get("authToPayment")
	if second.State != types.AssignmentPromoted || status.State != types.AssignmentPromoted || status.ActiveProfile != "QuantumSafeProfile" {
		t.Fatalf("after readiness the edge must be Promoted and active, got state=%s active=%q", status.State, status.ActiveProfile)
	}
}

// TestRuntimeDoesNotPromoteNewProfileBeforeReadiness checks that an already
// committed profile stays active while the new one waits for readiness.
func TestRuntimeDoesNotPromoteNewProfileBeforeReadiness(t *testing.T) {
	runtime := newGateRuntime(t)
	runtime.HealthObserver = &scriptedHealth{results: []mesh.EdgeHealth{mesh.EdgeHealthUnknown}}
	// Seed the store with an already committed assignment on the edge.
	runtime.Store.Upsert(types.CommunicationEdge{ID: "frontendToAuth", Namespace: "finance", ActiveProfile: "LegacyProfile", State: types.AssignmentPromoted})

	edge, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	status, _ := runtime.StatusStore.Get("frontendToAuth")
	if edge.State != types.AssignmentApplying || status.ActiveProfile != "LegacyProfile" {
		t.Fatalf("expected Applying with the previous active profile preserved, got state=%s active=%q", status.State, status.ActiveProfile)
	}
	if status.DesiredProfile != "HybridProfile" {
		t.Fatalf("expected desired HybridProfile, got %q", status.DesiredProfile)
	}
}

// TestRuntimeUnhealthyWorkloadPreservesPreviousProfile checks that an unhealthy
// reading fails the edge with reason workload-not-ready and leaves the previous
// active profile in place (G4).
func TestRuntimeUnhealthyWorkloadPreservesPreviousProfile(t *testing.T) {
	runtime := newGateRuntime(t)
	runtime.HealthObserver = &scriptedHealth{results: []mesh.EdgeHealth{mesh.EdgeHealthUnhealthy}}
	runtime.Store.Upsert(types.CommunicationEdge{ID: "frontendToAuth", Namespace: "finance", ActiveProfile: "LegacyProfile", State: types.AssignmentPromoted})

	edge, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if edge.State != types.AssignmentFailed || edge.Reason != "workload-not-ready" || edge.ActiveProfile != "LegacyProfile" {
		t.Fatalf("expected Failed/workload-not-ready with LegacyProfile preserved, got state=%s reason=%s active=%q", edge.State, edge.Reason, edge.ActiveProfile)
	}
}

// Threat gates

// TestRuntimeRejectsDeprecatedAndAdvisedProfiles checks that a runtime advisory
// against the only compatible profile blocks the edge with no-admissible-profile.
func TestRuntimeRejectsDeprecatedAndAdvisedProfiles(t *testing.T) {
	runtime := newGateRuntime(t)
	runtime.Resources.UpsertDeprecation("HybridProfile", "advisory:test")
	if _, err := runtime.ReconcileFixtureEdgeByServices(context.Background(), "finance-scenario.ttl", "Frontend", "Auth"); err == nil {
		t.Fatal("expected the only compatible profile to be rejected by the threat gate")
	}
	status, _ := runtime.StatusStore.Get("frontendToAuth")
	if status.State != types.AssignmentBlocked || status.Reason != "no-admissible-profile" {
		t.Fatalf("expected no-admissible-profile block, got state=%s reason=%s", status.State, status.Reason)
	}
}

// TestThreatGateCoversFamilyProofStatusAndAdvisory checks the failure reason of
// ThreatGateFailure for a clean profile, a deprecated profile, a deprecated
// family, insufficient proof status and an active advisory.
func TestThreatGateCoversFamilyProofStatusAndAdvisory(t *testing.T) {
	base := types.ProtocolProfile{Name: "p", Family: "fam", SecurityCategory: types.SecurityQuantumSafe}
	cases := []struct {
		mutate func(*types.ProtocolProfile)
		dep    map[string]bool
		want   string
	}{
		{func(p *types.ProtocolProfile) {}, nil, ""},
		{func(p *types.ProtocolProfile) { p.Deprecated = true }, nil, "deprecated-family"},
		{func(p *types.ProtocolProfile) {}, map[string]bool{"fam": true}, "deprecated-family"},
		{func(p *types.ProtocolProfile) { p.ProofStatus = "insufficient" }, nil, "insufficient-proof-status"},
		{func(p *types.ProtocolProfile) { p.Advisory = "ADV-1" }, nil, "active-advisory"},
	}
	for i, tc := range cases {
		p := base
		tc.mutate(&p)
		if got := orchestration.ThreatGateFailure(p, tc.dep); got != tc.want {
			t.Fatalf("case %d: got %q want %q", i, got, tc.want)
		}
	}
}

// TestHybridFallbackRequiresRobustCombiner checks that the policy-permitted hybrid
// fallback admits a hybrid profile only when it is marked robust (G1, the hybrid
// fallback clause of Thm. 1).
func TestHybridFallbackRequiresRobustCombiner(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "fallback",
		RequiredSecurityLevel: types.SecurityQuantumSafe,
		Policies:              []types.Policy{{Name: "transition", AllowHybrid: true}},
		CandidateProfiles:     []types.ProtocolProfile{{Name: "h", SecurityCategory: types.SecurityHybrid, OverheadScore: 1}},
	}
	if _, ok := orchestration.SelectProfile(edge, nil); ok {
		t.Fatal("a hybrid profile without recorded robustness must not be admitted by the fallback")
	}
	edge.CandidateProfiles[0].HybridRobust = true
	if sel, ok := orchestration.SelectProfile(edge, nil); !ok || sel.Reason != "policy-permitted-hybrid-fallback" {
		t.Fatalf("expected robust hybrid fallback, got ok=%v reason=%s", ok, sel.Reason)
	}
}

// Score configuration

// TestDefaultScoreConfigIsThePaperOperatingPoint checks the weights 0.25, 0.40,
// 0.20, 0.15 and the hysteresis 0.05 stated in Sect. 3.
func TestDefaultScoreConfigIsThePaperOperatingPoint(t *testing.T) {
	c := orchestration.DefaultScoreConfig
	if c.Wo != 0.25 || c.Wr != 0.40 || c.Wm != 0.20 || c.Wh != 0.15 || c.Delta != 0.05 {
		t.Fatalf("unexpected default score configuration %+v", c)
	}
}

// TestWeightedScoreChangesWinnerRelativeToUnweightedSum is a golden case for the
// ranking. An unweighted sum (hybrid 1.00 + 0.15 = 1.15, quantum 1.20) picks the
// cheaper hybrid profile. The weighting of Sect. 3 scores hybrid at
// 0.25*1.00 + 0.40*0.15 = 0.31 and quantum-safe at 0.25*1.20 = 0.30, which means it picks
// the quantum-safe profile. Both profiles are admissible, which shows that the
// weights only reorder the admissible set (Prop. 1).
func TestWeightedScoreChangesWinnerRelativeToUnweightedSum(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "golden",
		RequiredSecurityLevel: types.SecurityHybrid,
		TrustBoundary:         "internal",
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "hybrid", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.00},
			{Name: "quantum", SecurityCategory: types.SecurityQuantumSafe, OverheadScore: 1.20},
		},
	}
	unweighted := orchestration.ScoreConfig{Wo: 1, Wr: 1, Wm: 1, Wh: 1, Delta: 0.35}
	u, ok := orchestration.SelectProfileWith(edge, nil, unweighted)
	if !ok || u.DesiredProfile != "hybrid" {
		t.Fatalf("unweighted sum should pick hybrid, got %s", u.DesiredProfile)
	}
	w, ok := orchestration.SelectProfile(edge, nil)
	if !ok || w.DesiredProfile != "quantum" {
		t.Fatalf("weighted score should pick quantum, got %s", w.DesiredProfile)
	}
}

// TestHysteresisUsesConfiguredDelta checks that the switch margin delta decides
// whether an active profile is kept. With delta 0.05 the cheaper profile wins,
// and with delta 0.35 the active profile is kept.
func TestHysteresisUsesConfiguredDelta(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "hyst",
		RequiredSecurityLevel: types.SecurityHybrid,
		ActiveProfile:         "current",
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "current", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.80},
			{Name: "cheaper", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.00},
		},
	}
	// The score gap is 0.25*0.8 - (0.20*0.2 + 0.15*0.35) = 0.1075, which lies
	// between the two margins tested.
	if sel, _ := orchestration.SelectProfile(edge, nil); sel.DesiredProfile != "cheaper" {
		t.Fatalf("delta=0.05 should switch, got %s", sel.DesiredProfile)
	}
	wide := orchestration.DefaultScoreConfig
	wide.Delta = 0.35
	if sel, _ := orchestration.SelectProfileWith(edge, nil, wide); sel.DesiredProfile != "current" {
		t.Fatalf("delta=0.35 should keep the active profile, got %s", sel.DesiredProfile)
	}
}

// Pre-commit checker

// TestCheckPromotionMirrorsGates checks that CheckPromotion accepts an admissible
// hybrid promotion and reports mesh-not-ready, endpoint-support and
// security-class as the first failing gate in the respective situations. The
// last case checks that a hybrid profile below a quantum-safe requirement is
// rejected when no policy permits the fallback (G2).
func TestCheckPromotionMirrorsGates(t *testing.T) {
	edge := capabilityEdge(knownCaps("HybridProfile"), knownCaps("HybridProfile"))
	edge.Policies = []types.Policy{{Name: "p", AllowHybrid: true}}
	hy := types.ProtocolProfile{Name: "HybridProfile", SecurityCategory: types.SecurityHybrid, HybridRobust: true}
	ok := orchestration.PromotionFacts{GraphAvailable: true, FactAvailable: true, FactFresh: true, RolloutFeasible: true, MeshReady: true}
	if good, reason := orchestration.CheckPromotion(edge, hy, ok, nil); !good {
		t.Fatalf("expected admissible, got %s", reason)
	}
	notReady := ok
	notReady.MeshReady = false
	if good, reason := orchestration.CheckPromotion(edge, hy, notReady, nil); good || reason != "mesh-not-ready" {
		t.Fatalf("expected mesh-not-ready, got %v %s", good, reason)
	}
	qs := types.ProtocolProfile{Name: "QuantumSafeProfile", SecurityCategory: types.SecurityQuantumSafe}
	if good, reason := orchestration.CheckPromotion(edge, qs, ok, nil); good || reason != "endpoint-support" {
		t.Fatalf("expected endpoint-support failure, got %v %s", good, reason)
	}
	edge.RequiredSecurityLevel = types.SecurityQuantumSafe
	edge.Policies = nil
	if good, reason := orchestration.CheckPromotion(edge, hy, ok, nil); good || reason != "security-class" {
		t.Fatalf("expected security-class failure without fallback permission, got %v %s", good, reason)
	}
}
