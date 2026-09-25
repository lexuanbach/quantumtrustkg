package types

import "time"

// CapabilityProfile records what a workload advertises: the algorithms it
// implements and the profiles it supports. It is the capability evidence of
// Sect. 3 and carries the fields that every fact has in the Quantum Trust
// Graph, namely a Source, an ObservedAt time and a FreshnessWindow. The
// runtime turns fresh records into EndpointCapabilities and intersects the two
// endpoints of an edge to obtain C_e. Capability provenance is operational
// and is not bound to a signed identity, as the paper notes.
type CapabilityProfile struct {
	Name             string
	Namespace        string
	WorkloadName     string
	WorkloadSelector map[string]string
	Algorithms       []string
	Profiles         []string
	Source           string
	ObservedAt       time.Time
	FreshnessWindow  time.Duration
}

// IsFresh reports whether the observation is still inside its freshness window
// at time now. A record with no observation time or a non-positive window is
// never fresh, which is the fail-closed choice. The window ends at
// ObservedAt+FreshnessWindow, and an observation exactly at that instant is
// already stale.
func (c CapabilityProfile) IsFresh(now time.Time) bool {
	if c.ObservedAt.IsZero() || c.FreshnessWindow <= 0 {
		return false
	}
	return now.Before(c.ObservedAt.Add(c.FreshnessWindow))
}
