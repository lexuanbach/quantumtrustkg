package types

import "time"

// AssignmentState is the lifecycle state of the assignment of one edge. It is
// reported in the AssignmentStatus record (Sect. 3, Implementation). Pending
// means that the edge waits, for example for its rollout window. Validated means
// that QTPO selected a profile. Applying means that the mesh objects were
// written and the controller waits for both workloads to be ready. Promoted
// means that the profile is recorded as active. Blocked means that no profile
// is admissible or that evidence is missing, and the previous active profile
// stays in place. Rollback and Failed report a migration that was reverted or
// that could not be applied. The Coq model has no lifecycle. It distinguishes
// only promoted from blocked outcomes.
type AssignmentState string

const (
	AssignmentPending   AssignmentState = "Pending"
	AssignmentValidated AssignmentState = "Validated"
	AssignmentApplying  AssignmentState = "Applying"
	AssignmentPromoted  AssignmentState = "Promoted"
	AssignmentBlocked   AssignmentState = "Blocked"
	AssignmentRollback  AssignmentState = "Rollback"
	AssignmentFailed    AssignmentState = "Failed"
)

// EdgeProvenance is the evidence record for the facts joined on one edge. Source
// names the observer and ObservedAt is when it observed. FreshUntil is the end
// of the freshness window, that is, ObservedAt plus the window (300 s by default
// in the paper). A zero FreshUntil means that no freshness is known. ConflictNote
// is non-empty when a contradiction is recorded. Confidence is a descriptive
// label that no gate reads. The Coq record provenance keeps only the derived
// Booleans fact_available, fact_fresh and fact_conflict.
type EdgeProvenance struct {
	Source       string
	ObservedAt   time.Time
	FreshUntil   time.Time
	Confidence   string
	ConflictNote string
}

// EndpointCapabilities is the capability evidence for one endpoint of an edge:
// the profiles it advertises, taken from fresh capability records only. Known
// is false when no record exists for the endpoint (or every record is stale),
// in which case the endpoint supports no profile and the edge blocks.
type EndpointCapabilities struct {
	Known    bool
	Stale    bool
	Profiles []string
	Source   string
}

// Supports reports whether the endpoint advertises the named profile.
func (c EndpointCapabilities) Supports(profile string) bool {
	if !c.Known {
		return false
	}
	for _, p := range c.Profiles {
		if p == profile {
			return true
		}
	}
	return false
}

// CommunicationEdge is the runtime record of one call edge (s_i, s_j), and it
// corresponds to the Coq record edge. It carries everything QTPO needs for the
// decision. That is the trust boundary, the attached policies, the candidate
// profiles, the required class Req(e) with its reason, the provenance, and the
// desired and active profile. DesiredProfile is what QTPO selected and
// ActiveProfile is the last committed assignment. They differ while a promotion
// is in progress or after a block. State and Reason report the outcome.
type CommunicationEdge struct {
	ID                    string
	Namespace             string
	SourceService         string
	DestinationService    string
	TrustBoundary         string
	Policies              []Policy
	CandidateProfiles     []ProtocolProfile
	DesiredProfile        string
	ActiveProfile         string
	RequiredSecurityLevel SecurityCategory
	RequirementReason     string
	State                 AssignmentState
	Reason                string
	Provenance            EdgeProvenance
	LastEvaluatedAt       time.Time

	// SourceCapabilities and DestinationCapabilities hold the endpoint
	// capability evidence the runtime intersects to obtain the compatible
	// profiles C_e (profiles both endpoints support).
	SourceCapabilities      EndpointCapabilities
	DestinationCapabilities EndpointCapabilities
}
