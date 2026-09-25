package graph

import (
	"errors"
	"time"

	"quantumtrustkg/controller/pkg/types"
)

// ValidateEdge applies the evidence checks that need only the edge record. It
// returns the reason of the first failure as an error, or nil. The reasons are
// stale-facts when the freshness deadline is missing or passed (gate (iv)),
// conflicting-facts when a contradiction note is recorded, and then two
// structural failures, missing-required-security-level and
// no-candidate-profiles. The runtime blocks the edge on any error, which is the
// fail-closed behaviour of Theorem 2. The deadline instant itself counts as
// fresh here, as in state.IsAdmissible.
func ValidateEdge(edge types.CommunicationEdge, now time.Time) error {
	if edge.Provenance.FreshUntil.IsZero() || now.After(edge.Provenance.FreshUntil) {
		return errors.New("stale-facts")
	}
	if edge.Provenance.ConflictNote != "" {
		return errors.New("conflicting-facts")
	}
	if edge.RequiredSecurityLevel == "" {
		return errors.New("missing-required-security-level")
	}
	if len(edge.CandidateProfiles) == 0 {
		return errors.New("no-candidate-profiles")
	}
	return nil
}
