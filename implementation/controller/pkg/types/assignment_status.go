package types

import "time"

// AssignmentStatus is the structured outcome of reconciling one edge. It has the
// fields that the controller-owned AssignmentStatus resource exposes: the desired
// and the active profile, the state, the reason and the required class with the
// reason for it (Sect. 3, Implementation). It is the audit view of a decision.
// It carries no candidate list. The per-candidate reasons are in DecisionTrace.
type AssignmentStatus struct {
	ID                    string
	Namespace             string
	SourceService         string
	DestinationService    string
	DesiredProfile        string
	ActiveProfile         string
	RequiredSecurityLevel SecurityCategory
	RequirementReason     string
	State                 AssignmentState
	Reason                string
	LastEvaluatedAt       time.Time
}

// StatusFromEdge copies the outcome fields of an edge into an AssignmentStatus.
// The runtime calls it each time it records the status of an edge.
func StatusFromEdge(edge CommunicationEdge) AssignmentStatus {
	return AssignmentStatus{
		ID:                    edge.ID,
		Namespace:             edge.Namespace,
		SourceService:         edge.SourceService,
		DestinationService:    edge.DestinationService,
		DesiredProfile:        edge.DesiredProfile,
		ActiveProfile:         edge.ActiveProfile,
		RequiredSecurityLevel: edge.RequiredSecurityLevel,
		RequirementReason:     edge.RequirementReason,
		State:                 edge.State,
		Reason:                edge.Reason,
		LastEvaluatedAt:       edge.LastEvaluatedAt,
	}
}
