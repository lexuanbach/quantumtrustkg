package orchestration

import (
	"strings"

	"quantumtrustkg/controller/pkg/types"
)

// FilterByThreat applies hard gates (ii) and (iii) of FilterGates. It drops
// every candidate whose profile or primitive family is deprecated, either in
// the profile metadata or in the runtime deprecation set, whose proof status
// is insufficient, or that is targeted by an active advisory. The order of the
// input is preserved. The Coq model folds these conditions into the single
// flag profile_deprecated (threat_okb in formal/coq/Model.v).
func FilterByThreat(candidates []types.ProtocolProfile, deprecated map[string]bool) []types.ProtocolProfile {
	filtered := make([]types.ProtocolProfile, 0, len(candidates))
	for _, candidate := range candidates {
		if ThreatGateFailure(candidate, deprecated) != "" {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

// ThreatGateFailure returns the reason a candidate fails gates (ii) and (iii),
// or "" when it passes. The reasons are deprecated-family,
// insufficient-proof-status and active-advisory, tested in that order, and
// they appear in the decision trace. The proof statuses insufficient, withdrawn
// and broken fail the gate, and an empty status does not. The deprecated map
// may key on either a profile name or a family name.
func ThreatGateFailure(candidate types.ProtocolProfile, deprecated map[string]bool) string {
	if candidate.Deprecated || deprecated[candidate.Name] || (candidate.Family != "" && deprecated[candidate.Family]) {
		return "deprecated-family"
	}
	switch strings.ToLower(strings.TrimSpace(candidate.ProofStatus)) {
	case "insufficient", "withdrawn", "broken":
		return "insufficient-proof-status"
	}
	if strings.TrimSpace(candidate.Advisory) != "" {
		return "active-advisory"
	}
	return ""
}
