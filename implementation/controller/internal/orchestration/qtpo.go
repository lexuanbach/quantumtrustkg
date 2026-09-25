// Package orchestration implements the QTPO procedure of Sect. 3 (Algorithm 1)
// for one communication edge: filter the candidate profiles through the hard
// gates, rank the survivors by cost with hysteresis, and block the edge when
// nothing survives.
//
// The files split the procedure as follows. capability.go computes the
// compatible set C_e. policy_filter.go is gate (i), the class gate.
// threat_filter.go is gates (ii) and (iii). fallback.go and qtpo.go hold the
// guarded hybrid fallback and the selection itself. ranking.go and config.go
// are the cost model and its weights. admissibility.go is an independent
// checker that mirrors the Coq predicates. rollout.go covers rollout gate (v)
// and assignment states. trace.go records why each candidate was kept or
// dropped. The Coq counterpart of this package is formal/coq/QTPO.v, and the
// guarantees G1 to G4 in formal/coq/Guarantees.v are about that model.
//
// This file contains the selection function. The evidence gates (iv) on
// freshness and contradiction, graph availability and mesh readiness are
// applied by the callers before and after it, in internal/controllers.
package orchestration

import "quantumtrustkg/controller/pkg/types"

// SelectProfile runs the filter-then-rank selection of Algorithm 1 for one edge
// under DefaultScoreConfig. The candidates on the edge must already be the
// compatible set C_e (see CompatibleProfiles). The runtime guarantees that
// before it calls this function. The result is the edge with DesiredProfile,
// State and Reason filled in, and a Boolean that is false when the edge is
// blocked.
func SelectProfile(edge types.CommunicationEdge, deprecated map[string]bool) (types.CommunicationEdge, bool) {
	return SelectProfileWith(edge, deprecated, DefaultScoreConfig)
}

// SelectProfileWith is SelectProfile with an explicit score configuration. It
// is the entry point of the sensitivity sweeps over the weights and delta.
//
// The order of steps is the one of Algorithm 1. Gate (i) filters by the
// required class, then gates (ii) and (iii) filter by threat status. If no
// candidate is left and a policy on the edge allows it, the fallback branch
// retries with the hybrid profiles recorded as robust, again through the
// threat gates. Only the survivors are ranked. As a result cfg cannot admit a
// candidate that a gate removed (Proposition 1). An empty survivor set blocks
// the edge with the reason no-admissible-profile and the previous assignment is
// left untouched.
//
// The Reason field of the result tells the three success cases apart. The
// fallback case is flagged only when the chosen profile came from the
// fallback set.
func SelectProfileWith(edge types.CommunicationEdge, deprecated map[string]bool, cfg ScoreConfig) (types.CommunicationEdge, bool) {
	candidates := FilterByPolicies(edge.CandidateProfiles, edge.RequiredSecurityLevel)
	candidates = FilterByThreat(candidates, deprecated)
	usedHybridFallback := false
	if len(candidates) == 0 {
		if allowsHybridFallback(edge) {
			candidates = FilterByThreat(ComputeHybridFallback(edge.CandidateProfiles), deprecated)
			usedHybridFallback = len(candidates) > 0
		}
	}
	best, keptCurrent, ok := RankQTPO(edge, candidates, cfg)
	if !ok {
		blocked := MarkBlocked(edge, "no-admissible-profile")
		return blocked, false
	}
	edge.DesiredProfile = best.Name
	if usedHybridFallback && best.SecurityCategory == types.SecurityHybrid {
		edge.Reason = "policy-permitted-hybrid-fallback"
	} else if keptCurrent {
		edge.Reason = "stability-margin-retained-active-profile"
	} else {
		edge.Reason = "qtpo-selected-minimum-score-profile"
	}
	edge.State = types.AssignmentValidated
	return edge, true
}

// allowsHybridFallback is PolicyPermitsHybrid(e) of Algorithm 1 and
// policy_permits_hybrid in the Coq model. It holds when any attached policy
// sets AllowHybrid.
func allowsHybridFallback(edge types.CommunicationEdge) bool {
	for _, policy := range edge.Policies {
		if policy.AllowHybrid {
			return true
		}
	}
	return false
}
