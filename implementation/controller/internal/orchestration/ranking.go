package orchestration

import "quantumtrustkg/controller/pkg/types"

// RankLowestOverhead returns the candidate with the smallest OverheadScore and
// false when the list is empty. Ties keep the earliest candidate. It ignores
// risk and class. It is an overhead-only rule for comparison and is not part
// of QTPO. No production path calls it.
func RankLowestOverhead(candidates []types.ProtocolProfile) (types.ProtocolProfile, bool) {
	if len(candidates) == 0 {
		return types.ProtocolProfile{}, false
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.OverheadScore < best.OverheadScore {
			best = candidate
		}
	}
	return best, true
}

// RankQTPO is the ranking step of Algorithm 1. It returns the minimum-score
// candidate under cfg, with ties resolved in favour of the earlier candidate.
// If the edge's active profile is among the candidates and its score exceeds
// the best score by at most cfg.Delta, the active profile is kept. That is the
// hysteresis that stops an edge from flapping between profiles with similar
// cost. The second result reports whether the active profile was kept, and the
// third is false when the list is empty. The candidates must already have
// passed the hard gates. Cost is linear in the number of candidates.
func RankQTPO(edge types.CommunicationEdge, candidates []types.ProtocolProfile, cfg ScoreConfig) (types.ProtocolProfile, bool, bool) {
	if len(candidates) == 0 {
		return types.ProtocolProfile{}, false, false
	}

	best := candidates[0]
	bestScore := ScoreProfileWith(edge, best, cfg)
	for _, candidate := range candidates[1:] {
		score := ScoreProfileWith(edge, candidate, cfg)
		if score < bestScore {
			best = candidate
			bestScore = score
		}
	}

	current, ok := findCandidateByName(candidates, edge.ActiveProfile)
	if !ok {
		return best, false, true
	}
	currentScore := ScoreProfileWith(edge, current, cfg)
	if currentScore-bestScore <= cfg.Delta {
		return current, true, true
	}
	return best, false, true
}

// ScoreProfile scores a candidate under DefaultScoreConfig.
func ScoreProfile(edge types.CommunicationEdge, candidate types.ProtocolProfile) float64 {
	return ScoreProfileWith(edge, candidate, DefaultScoreConfig)
}

// ScoreProfileWith computes the cost of Sect. 3, namely
// Wo*Overhead + Wr*Risk + Wm*MigrationCost + Wh*SwitchPenalty. Lower is
// better. Overhead is the declared OverheadScore of the profile. The other
// three terms are computed below from the edge and its active profile.
func ScoreProfileWith(edge types.CommunicationEdge, candidate types.ProtocolProfile, cfg ScoreConfig) float64 {
	return cfg.Wo*candidate.OverheadScore +
		cfg.Wr*riskPenalty(edge, candidate) +
		cfg.Wm*migrationPenalty(edge, candidate) +
		cfg.Wh*switchPenalty(edge, candidate)
}

// boundarySensitivity is the exposure weight of a trust boundary. Regulated
// boundaries weigh most, partner and cross-zone boundaries less and internal
// (or unlabelled) boundaries least.
func boundarySensitivity(boundary string) float64 {
	switch boundary {
	case "regulated", "external-regulated":
		return 1.0
	case "partner", "external", "cross-zone":
		return 0.6
	default:
		return 0.3
	}
}

// riskPenalty is the residual-exposure term Risk of the score, clipped to
// [0,1]. A candidate below the required class scores 1. Gate (i) removes such
// a candidate before ranking. The value therefore appears only in decision
// traces.
// For a candidate at or above the required class, the term is its distance
// from quantum_safe (0 for quantum_safe, 0.5 for hybrid, 1 for classical)
// multiplied by the boundary sensitivity, plus the severity of any
// non-blocking advisory. A quantum_safe profile therefore has zero residual
// class risk everywhere, and a hybrid profile costs less on an internal edge
// than on a regulated one.
func riskPenalty(edge types.CommunicationEdge, candidate types.ProtocolProfile) float64 {
	required := edge.RequiredSecurityLevel
	if required != "" && securityRank(candidate.SecurityCategory) < securityRank(required) {
		return 1.0
	}
	residual := float64(securityRank(types.SecurityQuantumSafe)-securityRank(candidate.SecurityCategory)) / 2.0
	risk := boundarySensitivity(edge.TrustBoundary)*residual + candidate.AdvisorySeverity
	if risk > 1 {
		return 1
	}
	if risk < 0 {
		return 0
	}
	return risk
}

// migrationPenalty is the MigrationCost term. Changing away from the active
// profile costs 0.2. On a regulated edge a profile that is not quantum_safe adds
// 1.0, and on a partner edge a quantum_safe profile adds 0.15.
// The constants are fixed in this file and are not part of ScoreConfig.
func migrationPenalty(edge types.CommunicationEdge, candidate types.ProtocolProfile) float64 {
	penalty := 0.0
	if edge.ActiveProfile != "" && edge.ActiveProfile != candidate.Name {
		penalty += 0.2
	}
	switch edge.TrustBoundary {
	case "regulated":
		if candidate.SecurityCategory != types.SecurityQuantumSafe {
			penalty += 1.0
		}
	case "partner":
		if candidate.SecurityCategory == types.SecurityQuantumSafe {
			penalty += 0.15
		}
	}
	return penalty
}

// switchPenalty is the SwitchPenalty term. It is 0 for the active profile (or
// for an edge with no active profile) and 0.35 for any other candidate.
func switchPenalty(edge types.CommunicationEdge, candidate types.ProtocolProfile) float64 {
	if edge.ActiveProfile == "" || edge.ActiveProfile == candidate.Name {
		return 0
	}
	return 0.35
}

// findCandidateByName looks a candidate up by profile name. An empty name is
// never found.
func findCandidateByName(candidates []types.ProtocolProfile, name string) (types.ProtocolProfile, bool) {
	if name == "" {
		return types.ProtocolProfile{}, false
	}
	for _, candidate := range candidates {
		if candidate.Name == name {
			return candidate, true
		}
	}
	return types.ProtocolProfile{}, false
}
