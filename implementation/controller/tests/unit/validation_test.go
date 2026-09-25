// Tests of graph.ValidateEdge, the pre-selection metadata check (Thm. 2, G4).
//
// ValidateEdge implements the freshness and contradiction part of gate (iv) and the
// requirement that an edge has a required class. Each test builds one edge with a
// single defect and checks the reason string that the controller writes to the
// AssignmentStatus: stale-facts, missing-required-security-level and
// conflicting-facts. The reasons are the "failed gate as the reason" of Thm. 2.

package unit

import (
	"testing"
	"time"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/pkg/types"
)

// TestValidateEdgeRejectsStaleFacts checks that an expired freshness deadline is
// rejected as stale-facts.
func TestValidateEdgeRejectsStaleFacts(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID:                    "edge-stale",
		RequiredSecurityLevel: types.SecurityHybrid,
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "hybrid-profile", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.0},
		},
		Provenance: types.EdgeProvenance{
			FreshUntil: now.Add(-time.Minute),
		},
	}

	err := graph.ValidateEdge(edge, now)
	if err == nil {
		t.Fatal("expected stale validation error")
	}
	if err.Error() != "stale-facts" {
		t.Fatalf("expected stale-facts error, got %v", err)
	}
}

// TestValidateEdgeRejectsMissingSecurityLevel checks that an edge without a
// required class is rejected.
func TestValidateEdgeRejectsMissingSecurityLevel(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID: "edge-missing-level",
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "hybrid-profile", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.0},
		},
		Provenance: types.EdgeProvenance{
			FreshUntil: now.Add(time.Minute),
		},
	}

	err := graph.ValidateEdge(edge, now)
	if err == nil {
		t.Fatal("expected missing security level error")
	}
	if err.Error() != "missing-required-security-level" {
		t.Fatalf("expected missing-required-security-level error, got %v", err)
	}
}

// TestValidateEdgeRejectsConflictingFacts checks that a contradiction note is
// rejected as conflicting-facts.
func TestValidateEdgeRejectsConflictingFacts(t *testing.T) {
	now := time.Now().UTC()
	edge := types.CommunicationEdge{
		ID:                    "edge-conflict",
		RequiredSecurityLevel: types.SecurityHybrid,
		CandidateProfiles: []types.ProtocolProfile{
			{Name: "hybrid-profile", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.0},
		},
		Provenance: types.EdgeProvenance{
			FreshUntil:   now.Add(time.Minute),
			ConflictNote: "capability disagreement detected",
		},
	}

	err := graph.ValidateEdge(edge, now)
	if err == nil {
		t.Fatal("expected conflicting-facts validation error")
	}
	if err.Error() != "conflicting-facts" {
		t.Fatalf("expected conflicting-facts error, got %v", err)
	}
}
