// This file is the runtime's independent admissibility checker. It mirrors
// the Coq predicates direct_candidateb, fallback_candidateb and gates_okb of
// formal/coq/Model.v field for field. The controller runs it before it commits
// a promotion, which is the independent re-check that Sect. 3 describes after
// Theorem 2. The selector in qtpo.go and this checker are written separately
// on purpose. That separation lets the tests in tests/unit compare the two
// differentially. The correspondence with the Coq model is tested and has not
// been machine-checked.
package orchestration

import (
	"strings"

	"quantumtrustkg/controller/pkg/types"
)

// PromotionFacts holds the edge-level gate inputs of the Coq predicate
// gates_okb in formal/coq/Model.v. They are graph availability, availability,
// freshness and contradiction of the provenance, rollout feasibility and mesh
// readiness. The caller computes each Boolean from the live state of the edge.
// Freshness, for example, comes from the FreshUntil time of the provenance.
type PromotionFacts struct {
	GraphAvailable  bool // st_graph_available
	FactAvailable   bool // fact_available
	FactFresh       bool // fact_fresh
	FactConflict    bool // fact_conflict
	RolloutFeasible bool // rollout_feasible
	MeshReady       bool // mesh_ready
}

// CheckPromotion decides whether the profile may be committed on the edge. It
// returns true with an empty reason, or false with the name of the first
// failed check. The two Coq predicates it mirrors are:
//
//	direct   = endpoint_supportb && threat_okb && class_leb(Req, class) && gates_okb
//	fallback = endpoint_supportb && threat_okb && class = Hybrid &&
//	           hybrid_robust && policy_permits_hybrid && gates_okb
//
// Checks run in the order gates, endpoint support, threat and class. The
// runtime calls the function immediately before it records a profile as
// active. A false result blocks the edge and leaves the previous assignment in
// place, which is the fail-closed behaviour of Theorem 2. An empty required
// class never passes the direct branch. Such an edge can take only the
// hybrid fallback. Limitation: the Coq model has a single deprecation flag
// for the threat gate, while this checker also reads the deprecation set, the
// proof status and the advisory field of the profile (gates (ii) and (iii)).
func CheckPromotion(edge types.CommunicationEdge, profile types.ProtocolProfile, facts PromotionFacts, deprecated map[string]bool) (bool, string) {
	if reason := checkGates(facts); reason != "" {
		return false, reason
	}
	if !(edge.SourceCapabilities.Supports(profile.Name) && edge.DestinationCapabilities.Supports(profile.Name)) {
		return false, "endpoint-support"
	}
	if !checkerThreatOK(profile, deprecated) {
		return false, "threat"
	}
	if checkerRank(edge.RequiredSecurityLevel) <= checkerRank(profile.SecurityCategory) && edge.RequiredSecurityLevel != "" {
		return true, ""
	}
	if profile.SecurityCategory == types.SecurityHybrid && profile.HybridRobust && checkerPolicyPermitsHybrid(edge) {
		return true, ""
	}
	return false, "security-class"
}

// checkGates returns the reason of the first failed edge-level gate, or ""
// when all pass. The order is the one of gate_failure in formal/coq/QTPO.v.
func checkGates(f PromotionFacts) string {
	switch {
	case !f.GraphAvailable:
		return "graph-unavailable"
	case !f.FactAvailable:
		return "fact-unavailable"
	case !f.FactFresh:
		return "stale-facts"
	case f.FactConflict:
		return "conflicting-facts"
	case !f.RolloutFeasible:
		return "rollout-blocked"
	case !f.MeshReady:
		return "mesh-not-ready"
	}
	return ""
}

// checkerThreatOK implements gates (ii) and (iii) for the checker. A profile
// fails when it is deprecated by name or by family, when its recorded proof
// status is insufficient, withdrawn or broken, or when an advisory targets it.
// It repeats the logic of ThreatGateFailure in threat_filter.go independently.
func checkerThreatOK(p types.ProtocolProfile, deprecated map[string]bool) bool {
	if p.Deprecated || deprecated[p.Name] {
		return false
	}
	if p.Family != "" && deprecated[p.Family] {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(p.ProofStatus))
	if status == "insufficient" || status == "withdrawn" || status == "broken" {
		return false
	}
	return strings.TrimSpace(p.Advisory) == ""
}

// checkerRank is the rank of a class in the order classical, hybrid and
// quantum_safe, as class_rank in the Coq model. An unknown or empty class has
// rank 0, below every declared class.
func checkerRank(c types.SecurityCategory) int {
	switch c {
	case types.SecurityClassical:
		return 1
	case types.SecurityHybrid:
		return 2
	case types.SecurityQuantumSafe:
		return 3
	}
	return 0
}

// checkerPolicyPermitsHybrid is policy_permits_hybrid of the Coq model. It is
// true when any policy attached to the edge explicitly allows the fallback.
func checkerPolicyPermitsHybrid(edge types.CommunicationEdge) bool {
	for _, p := range edge.Policies {
		if p.AllowHybrid {
			return true
		}
	}
	return false
}
