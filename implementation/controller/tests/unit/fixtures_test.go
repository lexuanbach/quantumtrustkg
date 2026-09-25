// Fixture parsing tests (Sect. 3, semantic input of the graph).
//
// The finance and Online Boutique fixtures in implementation/semantics/fixtures
// are Turtle files that describe services, profiles, policies and communications.
// graph.BuildEdgesFromTTL turns them into communication edges. These tests check
// that the parse yields the edges, the required class (with the reason string that
// inference produces), the candidate profiles and the provenance freshness field
// that the decisions of Alg. 1 consume. They also check that the generated 500- and
// 1000-service fixtures parse to the expected size, which the scale-stress command
// depends on. No guarantee is checked here directly. The tests protect the inputs
// that G1, G2 and G4 are stated over.

package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/pkg/types"
)

// TestBuildFinanceDemoEdgeFromTTL checks the demo edge authToPayment: endpoints,
// a quantum-safe requirement and exactly one candidate, QuantumSafeProfile.
func TestBuildFinanceDemoEdgeFromTTL(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", "finance-scenario.ttl")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	edge, err := graph.BuildFinanceDemoEdgeFromTTL(raw, "authToPayment")
	if err != nil {
		t.Fatalf("build edge: %v", err)
	}

	if edge.SourceService != "Auth" || edge.DestinationService != "Payment" {
		t.Fatalf("unexpected edge endpoints: %s -> %s", edge.SourceService, edge.DestinationService)
	}
	if edge.RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected quantum_safe requirement, got %s", edge.RequiredSecurityLevel)
	}
	if len(edge.CandidateProfiles) != 1 {
		t.Fatalf("expected exactly 1 protocol candidate, got %d", len(edge.CandidateProfiles))
	}
	if edge.CandidateProfiles[0].Name != "QuantumSafeProfile" {
		t.Fatalf("expected QuantumSafeProfile, got %s", edge.CandidateProfiles[0].Name)
	}
}

// TestBuildEdgesFromTTL checks all three finance edges. The frontend edge
// requires hybrid, the two PCI edges require quantum-safe, and the requirement
// reason lists the policy, the PCI inference and the trust boundary.
func TestBuildEdgesFromTTL(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", "finance-scenario.ttl")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	edges, err := graph.BuildEdgesFromTTL(raw)
	if err != nil {
		t.Fatalf("build edges: %v", err)
	}

	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(edges))
	}

	byID := make(map[string]types.CommunicationEdge, len(edges))
	for _, edge := range edges {
		byID[edge.ID] = edge
	}

	if byID["frontendToAuth"].RequiredSecurityLevel != types.SecurityHybrid {
		t.Fatalf("expected frontendToAuth to require hybrid, got %s", byID["frontendToAuth"].RequiredSecurityLevel)
	}
	if byID["authToPayment"].RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected authToPayment to require quantum_safe, got %s", byID["authToPayment"].RequiredSecurityLevel)
	}
	if byID["paymentToLedger"].RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected paymentToLedger to require quantum_safe, got %s", byID["paymentToLedger"].RequiredSecurityLevel)
	}
	if byID["authToPayment"].RequirementReason == "" {
		t.Fatal("expected inferred requirement reason to be populated")
	}
	if byID["authToPayment"].RequirementReason != "policy:PCIPolicy,inference:pci-regulated-edge,boundary:regulated" {
		t.Fatalf("unexpected requirement reason: %s", byID["authToPayment"].RequirementReason)
	}
}

// TestBuildStaleEdgeFromTTL checks that the stale fixture carries a freshness
// deadline. The runtime tests use this fixture to exercise the fail-closed path.
func TestBuildStaleEdgeFromTTL(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", "stale-finance-scenario.ttl")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	edge, err := graph.BuildFinanceDemoEdgeFromTTL(raw, "authToPayment")
	if err != nil {
		t.Fatalf("build edge: %v", err)
	}

	if edge.Provenance.FreshUntil.IsZero() {
		t.Fatal("expected stale fixture to contain provenance freshness data")
	}
}

// TestGeneratedScaleFixturesParse checks the service and edge counts of the
// generated scale fixtures (two edges per service) and that edges expose
// candidate profiles.
func TestGeneratedScaleFixturesParse(t *testing.T) {
	cases := []struct {
		name     string
		services int
		edges    int
	}{
		{name: "scale-500.ttl", services: 500, edges: 1000},
		{name: "scale-1000.ttl", services: 1000, edges: 2000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", tc.name)
			raw, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			if got := countServiceDeclarations(raw); got != tc.services {
				t.Fatalf("expected %d services, got %d", tc.services, got)
			}

			edges, err := graph.BuildEdgesFromTTL(raw)
			if err != nil {
				t.Fatalf("build edges: %v", err)
			}
			if len(edges) != tc.edges {
				t.Fatalf("expected %d edges, got %d", tc.edges, len(edges))
			}
			if len(edges[0].CandidateProfiles) == 0 {
				t.Fatal("expected generated fixture edges to expose candidate profiles")
			}
		})
	}
}

// countServiceDeclarations counts the lines that declare a :Service.
func countServiceDeclarations(raw []byte) int {
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(strings.TrimSpace(line), " a :Service") {
			count++
		}
	}
	return count
}
