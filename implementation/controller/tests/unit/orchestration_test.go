// Unit tests of the selection procedure in isolation (Sect. 3, Alg. 1).
//
// These tests call orchestration.SelectProfile and ComputeRolloutWindow on small
// hand-built edges, without a runtime. They check the filter-before-rank order
// (the admissible profile with the lowest score wins), the stability margin that
// keeps the active profile unless a candidate is clearly better, the
// policy-permitted hybrid fallback with its reason string, the block on an empty
// candidate list (G4) and the canary window that starts with lower-risk edges.

package unit

import (
	"testing"
	"time"

	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// TestSelectProfilePrefersAdmissibleLowestOverhead checks that among two
// admissible profiles the cheaper one is selected and the edge is Validated.
func TestSelectProfilePrefersAdmissibleLowestOverhead(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID:                    "edge-1",
		RequiredSecurityLevel: types.SecurityHybrid,
		CandidateProfiles: []types.ProtocolProfile{
			{
				Name:             "quantum-safe-profile",
				SecurityCategory: types.SecurityQuantumSafe,
				OverheadScore:    1.5,
			},
			{
				Name:             "hybrid-profile",
				SecurityCategory: types.SecurityHybrid,
				OverheadScore:    1.1,
			},
		},
		Provenance: types.EdgeProvenance{
			ObservedAt: now,
			FreshUntil: now.Add(time.Minute),
		},
	}

	selected, ok := orchestration.SelectProfile(edge, map[string]bool{})
	if !ok {
		t.Fatal("expected profile selection to succeed")
	}
	if selected.DesiredProfile != "hybrid-profile" {
		t.Fatalf("expected hybrid-profile, got %s", selected.DesiredProfile)
	}
	if selected.State != types.AssignmentValidated {
		t.Fatalf("expected validated state, got %s", selected.State)
	}
}

// TestSelectProfileRetainsCurrentProfileWithinStabilityMargin checks that a small
// improvement does not displace the active profile.
func TestSelectProfileRetainsCurrentProfileWithinStabilityMargin(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID:                    "edge-stable",
		RequiredSecurityLevel: types.SecurityHybrid,
		ActiveProfile:         "hybrid-current",
		CandidateProfiles: []types.ProtocolProfile{
			{
				Name:             "hybrid-current",
				SecurityCategory: types.SecurityHybrid,
				OverheadScore:    1.20,
			},
			{
				Name:             "hybrid-alternative",
				SecurityCategory: types.SecurityHybrid,
				OverheadScore:    1.05,
			},
		},
		Provenance: types.EdgeProvenance{
			ObservedAt: now,
			FreshUntil: now.Add(time.Minute),
		},
	}

	selected, ok := orchestration.SelectProfile(edge, map[string]bool{})
	if !ok {
		t.Fatal("expected profile selection to succeed")
	}
	if selected.DesiredProfile != "hybrid-current" {
		t.Fatalf("expected current profile to be retained, got %s", selected.DesiredProfile)
	}
	if selected.Reason != "stability-margin-retained-active-profile" {
		t.Fatalf("unexpected reason: %s", selected.Reason)
	}
}

// TestSelectProfileSwitchesWhenImprovementExceedsStabilityMargin checks that a
// large improvement does displace it.
func TestSelectProfileSwitchesWhenImprovementExceedsStabilityMargin(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID:                    "edge-switch",
		RequiredSecurityLevel: types.SecurityHybrid,
		ActiveProfile:         "hybrid-current",
		CandidateProfiles: []types.ProtocolProfile{
			{
				Name:             "hybrid-current",
				SecurityCategory: types.SecurityHybrid,
				OverheadScore:    1.60,
			},
			{
				Name:             "hybrid-alternative",
				SecurityCategory: types.SecurityHybrid,
				OverheadScore:    0.70,
			},
		},
		Provenance: types.EdgeProvenance{
			ObservedAt: now,
			FreshUntil: now.Add(time.Minute),
		},
	}

	selected, ok := orchestration.SelectProfile(edge, map[string]bool{})
	if !ok {
		t.Fatal("expected profile selection to succeed")
	}
	if selected.DesiredProfile != "hybrid-alternative" {
		t.Fatalf("expected profile switch, got %s", selected.DesiredProfile)
	}
}

// TestSelectProfileFlagsPolicyPermittedHybridFallback checks the hybrid fallback
// under a quantum-safe requirement when the policy allows hybrid.
func TestSelectProfileFlagsPolicyPermittedHybridFallback(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "edge-fallback",
		RequiredSecurityLevel: types.SecurityQuantumSafe,
		Policies: []types.Policy{{
			Name:        "pci-transition",
			AllowHybrid: true,
		}},
		CandidateProfiles: []types.ProtocolProfile{{
			Name:             "hybrid-profile",
			SecurityCategory: types.SecurityHybrid,
			OverheadScore:    1.0,
			HybridRobust:     true,
		}},
	}

	selected, ok := orchestration.SelectProfile(edge, map[string]bool{})
	if !ok {
		t.Fatal("expected policy-permitted hybrid fallback to succeed")
	}
	if selected.DesiredProfile != "hybrid-profile" {
		t.Fatalf("expected hybrid fallback, got %s", selected.DesiredProfile)
	}
	if selected.Reason != "policy-permitted-hybrid-fallback" {
		t.Fatalf("expected fallback reason, got %s", selected.Reason)
	}
}

// TestSelectProfileBlocksWhenNoCandidateExists checks the fail-closed outcome:
// a Blocked state with a non-empty reason.
func TestSelectProfileBlocksWhenNoCandidateExists(t *testing.T) {
	edge := types.CommunicationEdge{
		ID:                    "edge-2",
		RequiredSecurityLevel: types.SecurityQuantumSafe,
		CandidateProfiles:     nil,
	}

	selected, ok := orchestration.SelectProfile(edge, map[string]bool{})
	if ok {
		t.Fatal("expected selection to fail")
	}
	if selected.State != types.AssignmentBlocked {
		t.Fatalf("expected blocked state, got %s", selected.State)
	}
	if selected.Reason == "" {
		t.Fatal("expected blocked reason to be populated")
	}
}

// TestComputeRolloutWindowPrefersLowerRiskEdgesForCanary checks that a canary
// window contains the internal edge and excludes the regulated one.
func TestComputeRolloutWindowPrefersLowerRiskEdgesForCanary(t *testing.T) {
	edges := []types.CommunicationEdge{
		{ID: "regulated-edge", TrustBoundary: "regulated"},
		{ID: "internal-edge", TrustBoundary: "internal"},
		{ID: "partner-edge", TrustBoundary: "partner"},
	}

	window := orchestration.ComputeRolloutWindow(edges, orchestration.RolloutConfig{
		Strategy: "canary",
	})

	if !window["internal-edge"] {
		t.Fatal("expected internal edge to be chosen for canary rollout")
	}
	if window["regulated-edge"] {
		t.Fatal("did not expect regulated edge in canary rollout window")
	}
}
