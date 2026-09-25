package orchestration

import "quantumtrustkg/controller/pkg/types"

// ComputeHybridFallback returns the candidates that the guarded hybrid
// fallback may use. A candidate qualifies when its declared class is hybrid
// and its combiner is recorded as robust. These are the class and robustness
// conditions of fallback_candidateb in formal/coq/Model.v, and guarantee G3
// in formal/coq/Guarantees.v states them for every fallback promotion.
//
// The function does not test whether policy permits the fallback. The caller
// (SelectProfileWith) checks that first, and the threat gates run on the
// result afterwards.
func ComputeHybridFallback(candidates []types.ProtocolProfile) []types.ProtocolProfile {
	filtered := make([]types.ProtocolProfile, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.SecurityCategory == types.SecurityHybrid && candidate.HybridRobust {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}
