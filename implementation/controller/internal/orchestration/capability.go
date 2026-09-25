package orchestration

import "quantumtrustkg/controller/pkg/types"

// CompatibleProfiles computes the compatible set C_e of Algorithm 1 (Sect. 3),
// the profiles that both endpoints of the edge advertise. The evidence must
// come from fresh capability records. A missing or stale record for either
// endpoint yields no candidates and the reason missing-capability:<svc> or
// stale-capability:<svc>. An empty intersection yields no-common-profile. This
// is the fail-closed reading of missing evidence and it corresponds to
// supports_profileb in formal/coq/Model.v, where an unknown service supports
// nothing. The caller must assign the result to the edge even when it is
// empty. Otherwise a stale candidate list would survive a failed intersection.
//
// Source endpoint evidence is checked before destination evidence. Only the
// first failure is reported.
func CompatibleProfiles(edge types.CommunicationEdge) ([]types.ProtocolProfile, string) {
	if reason := endpointEvidenceFailure(edge.SourceCapabilities, edge.SourceService); reason != "" {
		return nil, reason
	}
	if reason := endpointEvidenceFailure(edge.DestinationCapabilities, edge.DestinationService); reason != "" {
		return nil, reason
	}
	compatible := make([]types.ProtocolProfile, 0, len(edge.CandidateProfiles))
	for _, profile := range edge.CandidateProfiles {
		if edge.SourceCapabilities.Supports(profile.Name) && edge.DestinationCapabilities.Supports(profile.Name) {
			compatible = append(compatible, profile)
		}
	}
	if len(compatible) == 0 {
		return nil, "no-common-profile"
	}
	return compatible, ""
}

// endpointEvidenceFailure names the evidence problem of one endpoint, or
// returns "" when the endpoint has usable capability evidence.
func endpointEvidenceFailure(c types.EndpointCapabilities, service string) string {
	if c.Known {
		return ""
	}
	if c.Stale {
		return "stale-capability:" + service
	}
	return "missing-capability:" + service
}
