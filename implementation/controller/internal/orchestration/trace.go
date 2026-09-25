// This file builds the decision trace of one edge, the auditable record of
// why QTPO chose a profile or blocked. Sect. 3 says that a failing candidate is
// dropped with its failed gate logged, and the trace is the machine-readable
// form of that log. It lists the edge, its required class and the reason for
// it, the policies and provenance source that applied, the selected profile,
// and for each candidate its class, its cost score and the first stage that
// rejected it. The cmd/decision-trace tool prints these records as JSON.
//
// Traces are deterministic. Scores are rounded to four decimals and candidates
// are sorted by name. The same input then gives byte-identical output on every
// platform. The trace is recomputed from the edge and does not read runtime
// state. It re-runs the same filters and SelectProfile as the controller. It
// therefore reports the decision that the controller would take on this input.
//
// Limitation: a rejected candidate carries one of three coarse reasons. The
// finer gate reasons of ThreatGateFailure (deprecated-family,
// insufficient-proof-status, active-advisory) are not copied into the trace.
package orchestration

import (
	"math"
	"sort"
	"strings"
	"time"

	"quantumtrustkg/controller/pkg/types"
)

// CandidateTrace is the outcome for one candidate profile. Score is the cost
// under DefaultScoreConfig. Rejected is empty for the selected profile and,
// when a profile was selected, names the stage that removed each other
// candidate: policy-or-required-class for gate (i), threat-or-deprecated-family
// for gates (ii) and (iii), and ranked-lower-than-selected for a candidate that
// passed the gates and lost the ranking. When the edge is blocked, candidates
// that passed both gates carry no reason.
type CandidateTrace struct {
	Name     string  `json:"name"`
	Class    string  `json:"class"`
	Score    float64 `json:"score"`
	Rejected string  `json:"rejected,omitempty"`
}

// DecisionTrace is the JSON record of one edge decision. RequirementReason lists
// what produced the required class, for example a policy name or the trust
// boundary. StatusReason is the Reason of the resulting assignment, and it also
// holds the block reason when no profile was selected. The two mesh names are
// the PeerAuthentication and DestinationRule objects that the edge would
// receive, derived from the edge ID the same way as in the mesh package.
type DecisionTrace struct {
	EdgeID                 string           `json:"edge_id"`
	SourceService          string           `json:"source_service"`
	DestinationService     string           `json:"destination_service"`
	RequiredClass          string           `json:"required_class"`
	RequirementReason      string           `json:"requirement_reason"`
	ActiveProfile          string           `json:"active_profile"`
	SelectedProfile        string           `json:"selected_profile"`
	StatusReason           string           `json:"status_reason"`
	Policies               []string         `json:"policies"`
	ProvenanceSource       string           `json:"provenance_source"`
	ProvenanceFreshUntil   string           `json:"provenance_fresh_until"`
	Candidates             []CandidateTrace `json:"candidates"`
	PeerAuthenticationName string           `json:"peer_authentication_name"`
	DestinationRuleName    string           `json:"destination_rule_name"`
}

// TraceSelection replays the selection for one edge and returns its trace. The
// deprecated map is the runtime deprecation set, as in SelectProfile. The
// candidate list should be the compatible set C_e. A profile taken through the
// hybrid fallback is reported with the fallback reason in StatusReason, but its
// candidate row is still marked by the class gate because the row logic does
// not model the fallback branch.
func TraceSelection(edge types.CommunicationEdge, deprecated map[string]bool) DecisionTrace {
	trace := DecisionTrace{
		EdgeID:                 edge.ID,
		SourceService:          edge.SourceService,
		DestinationService:     edge.DestinationService,
		RequiredClass:          string(edge.RequiredSecurityLevel),
		RequirementReason:      edge.RequirementReason,
		ActiveProfile:          edge.ActiveProfile,
		Policies:               policyNames(edge.Policies),
		ProvenanceSource:       edge.Provenance.Source,
		ProvenanceFreshUntil:   formatTraceTime(edge.Provenance.FreshUntil),
		PeerAuthenticationName: sanitizeTraceName(edge.ID + "-mtls"),
		DestinationRuleName:    sanitizeTraceName(edge.ID + "-tls"),
	}

	policyCandidates := FilterByPolicies(edge.CandidateProfiles, edge.RequiredSecurityLevel)
	threatCandidates := FilterByThreat(policyCandidates, deprecated)
	selected, ok := SelectProfile(edge, deprecated)
	if ok {
		trace.SelectedProfile = selected.DesiredProfile
		trace.StatusReason = selected.Reason
	} else {
		trace.StatusReason = selected.Reason
	}

	for _, candidate := range edge.CandidateProfiles {
		item := CandidateTrace{
			Name:  candidate.Name,
			Class: string(candidate.SecurityCategory),
			// Rounded to 4 decimals so the trace is stable across platforms.
			Score: math.Round(ScoreProfile(edge, candidate)*1e4) / 1e4,
		}
		switch {
		case !containsProfile(policyCandidates, candidate.Name):
			item.Rejected = "policy-or-required-class"
		case !containsProfile(threatCandidates, candidate.Name):
			item.Rejected = "threat-or-deprecated-family"
		case trace.SelectedProfile != "" && candidate.Name != trace.SelectedProfile:
			item.Rejected = "ranked-lower-than-selected"
		}
		trace.Candidates = append(trace.Candidates, item)
	}
	// Candidate profiles arrive in map-derived order. Sorting by name makes
	// the trace deterministic and byte-comparable across runs.
	sort.SliceStable(trace.Candidates, func(i, j int) bool {
		return trace.Candidates[i].Name < trace.Candidates[j].Name
	})
	return trace
}

// policyNames lists the names of the attached policies in their given order.
func policyNames(policies []types.Policy) []string {
	names := make([]string, 0, len(policies))
	for _, policy := range policies {
		names = append(names, policy.Name)
	}
	return names
}

// containsProfile reports whether a profile with the given name is present.
func containsProfile(candidates []types.ProtocolProfile, name string) bool {
	for _, candidate := range candidates {
		if candidate.Name == name {
			return true
		}
	}
	return false
}

// formatTraceTime formats a time as UTC RFC 3339, or as "" for a zero time.
func formatTraceTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// sanitizeTraceName replaces underscores, spaces, slashes and colons in a name
// with hyphens. It mirrors the name sanitizer of the mesh package.
func sanitizeTraceName(in string) string {
	replacer := strings.NewReplacer("_", "-", " ", "-", "/", "-", ":", "-")
	return replacer.Replace(in)
}
