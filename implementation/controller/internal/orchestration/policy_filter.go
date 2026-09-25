package orchestration

import "quantumtrustkg/controller/pkg/types"

// FilterByPolicies implements hard gate (i) of FilterGates. It keeps the
// candidates whose declared class is at least the required class Req(e).
// The classes are ordered classical, hybrid and quantum_safe. A required
// class that is empty ranks as classical and admits every candidate. The
// caller therefore sets a default requirement before it filters (the graph fixtures use
// hybrid).
func FilterByPolicies(candidates []types.ProtocolProfile, required types.SecurityCategory) []types.ProtocolProfile {
	filtered := make([]types.ProtocolProfile, 0, len(candidates))
	for _, candidate := range candidates {
		if securityRank(candidate.SecurityCategory) >= securityRank(required) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

// securityRank orders the classes for gate (i) and for the risk term of the
// score. It is offset by one against class_rank in the Coq model and induces
// the same order. An unknown class ranks as classical.
func securityRank(category types.SecurityCategory) int {
	switch category {
	case types.SecurityQuantumSafe:
		return 2
	case types.SecurityHybrid:
		return 1
	default:
		return 0
	}
}
