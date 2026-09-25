// Tests on the Online Boutique fixture (extended version, Online Boutique
// appendix).
//
// The fixture holds the 11 managed edges of the Online Boutique demo. The edge from
// Checkout to Payment is the worked example of the paper. It is regulated, requires
// quantum-safe protection and offers a hybrid and a quantum-safe candidate. These
// tests check the fixture parse and the decision trace of that edge, which is the
// record that cmd/decision-trace writes to
// online-boutique-decision-trace.json. The hybrid candidate must be rejected at the
// policy and class gate (gate (i) of the QTPO filter), and the trace must name the
// PeerAuthentication and DestinationRule of the edge.

package unit

import (
	"os"
	"path/filepath"
	"testing"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// TestOnlineBoutiqueFixtureParsesDecisionEdge checks the edge count, the required
// class and the two candidates of Checkout to Payment.
func TestOnlineBoutiqueFixtureParsesDecisionEdge(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", "online-boutique.ttl")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	edges, err := graph.BuildEdgesFromTTL(raw)
	if err != nil {
		t.Fatalf("build edges: %v", err)
	}
	if len(edges) != 11 {
		t.Fatalf("expected 11 Online Boutique edges, got %d", len(edges))
	}
	edge := findEdge(t, edges, "Checkout", "Payment")
	if edge.ID != "checkoutToPayment" {
		t.Fatalf("unexpected edge ID: %s", edge.ID)
	}
	if edge.RequiredSecurityLevel != types.SecurityQuantumSafe {
		t.Fatalf("expected quantum_safe requirement, got %s", edge.RequiredSecurityLevel)
	}
	if len(edge.CandidateProfiles) != 2 {
		t.Fatalf("expected hybrid and quantum_safe candidates, got %d", len(edge.CandidateProfiles))
	}
}

// TestOnlineBoutiqueDecisionTraceCompleteness checks the selected profile, the
// generated object names and the rejection of the hybrid candidate.
func TestOnlineBoutiqueDecisionTraceCompleteness(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "semantics", "fixtures", "online-boutique.ttl")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	edges, err := graph.BuildEdgesFromTTL(raw)
	if err != nil {
		t.Fatalf("build edges: %v", err)
	}
	trace := orchestration.TraceSelection(findEdge(t, edges, "Checkout", "Payment"), map[string]bool{})
	if trace.SelectedProfile != "QuantumSafeProfile" {
		t.Fatalf("expected QuantumSafeProfile, got %s", trace.SelectedProfile)
	}
	if trace.PeerAuthenticationName != "checkoutToPayment-mtls" || trace.DestinationRuleName != "checkoutToPayment-tls" {
		t.Fatalf("missing generated object names: %#v", trace)
	}
	if len(trace.Candidates) != 2 {
		t.Fatalf("expected 2 traced candidates, got %d", len(trace.Candidates))
	}
	foundPolicyRejection := false
	for _, candidate := range trace.Candidates {
		if candidate.Name == "HybridProfile" && candidate.Rejected == "policy-or-required-class" {
			foundPolicyRejection = true
		}
	}
	if !foundPolicyRejection {
		t.Fatal("expected hybrid candidate to be rejected by policy/class gate")
	}
}

// findEdge returns the edge between two services or fails the test.
func findEdge(t *testing.T, edges []types.CommunicationEdge, source, destination string) types.CommunicationEdge {
	t.Helper()
	for _, edge := range edges {
		if edge.SourceService == source && edge.DestinationService == destination {
			return edge
		}
	}
	t.Fatalf("edge %s -> %s not found", source, destination)
	return types.CommunicationEdge{}
}
