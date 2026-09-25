// Cluster inventory: builds communication edges from Kubernetes Services
// (Sect. 3, implementation).
//
// In a live cluster the call graph comes from the annotation
// quantumtrustkg.io/calls (a comma-separated list of callee names) on each
// Service, and the trust boundary from quantumtrustkg.io/trust-boundary
// (default "internal"). This is the annotation-only topology source that the
// discussion (Sect. 5) lists as a limitation. For every listed call the source
// builds a CommunicationEdge whose candidate profiles are the intersection of the
// profiles that the CryptoCapabilityProfile records advertise for the two
// endpoints. Provenance is the inventory itself, valid for five minutes.
//
// The security levels of the candidate profiles are inferred from the profile
// name (see inventoryProfile). The required class set here is only an initial
// value. AssignmentRuntime recomputes it from policies and the trust boundary
// before selection.

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/pkg/types"
)

// InventorySource yields the edges of a namespace. It is an interface so tests
// can supply a fixed edge set without a cluster.
type InventorySource interface {
	BuildEdges(ctx context.Context, namespace string) ([]types.CommunicationEdge, error)
}

// ClusterInventorySource lists Services through the controller-runtime client.
type ClusterInventorySource struct {
	Client    ctrlclient.Client
	Resources *ResourceState
}

// NewClusterInventorySource returns a source that reads Services with client and
// resolves capabilities from resources.
func NewClusterInventorySource(client ctrlclient.Client, resources *ResourceState) *ClusterInventorySource {
	return &ClusterInventorySource{Client: client, Resources: resources}
}

// BuildEdges returns one edge per annotated call of every Service in the
// namespace. It also records each Service's labels in ResourceState, because
// policy and migration selectors match on them. The edge ID is the
// concatenation SourceToDestination.
func (s *ClusterInventorySource) BuildEdges(ctx context.Context, namespace string) ([]types.CommunicationEdge, error) {
	if s == nil || s.Client == nil {
		return nil, fmt.Errorf("cluster inventory client is nil")
	}

	services := &corev1.ServiceList{}
	if err := s.Client.List(ctx, services, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}

	edges := make([]types.CommunicationEdge, 0)
	now := time.Now().UTC()
	for _, service := range services.Items {
		if s.Resources != nil {
			s.Resources.UpsertServiceLabels(namespace, service.Name, service.Labels)
		}
		calls := splitCalls(service.Annotations["quantumtrustkg.io/calls"])
		for _, dest := range calls {
			edge := types.CommunicationEdge{
				ID:                    service.Name + "To" + dest,
				Namespace:             namespace,
				SourceService:         service.Name,
				DestinationService:    dest,
				TrustBoundary:         edgeBoundary(service.Annotations),
				State:                 types.AssignmentPending,
				RequiredSecurityLevel: types.SecurityHybrid,
				Provenance: types.EdgeProvenance{
					Source:     "cluster-inventory",
					ObservedAt: now,
					FreshUntil: now.Add(5 * time.Minute),
					Confidence: "medium",
				},
			}
			edge.CandidateProfiles = inventoryCandidateProfiles(namespace, service.Name, dest, s.Resources)
			edges = append(edges, edge)
		}
	}
	return edges, nil
}

// splitCalls parses the comma-separated calls annotation and drops empty items.
func splitCalls(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	calls := make([]string, 0, len(parts))
	for _, part := range parts {
		call := strings.TrimSpace(part)
		if call != "" {
			calls = append(calls, call)
		}
	}
	return calls
}

// edgeBoundary returns the trust-boundary annotation or "internal" if absent.
func edgeBoundary(annotations map[string]string) string {
	if annotations == nil {
		return "internal"
	}
	if boundary := strings.TrimSpace(annotations["quantumtrustkg.io/trust-boundary"]); boundary != "" {
		return boundary
	}
	return "internal"
}

// inventoryCandidateProfiles returns the profiles that both endpoints
// advertise. It is empty when either side has no matching capability record,
// and the runtime then blocks the edge because no profile is supported by both
// ends.
func inventoryCandidateProfiles(namespace, source, destination string, resources *ResourceState) []types.ProtocolProfile {
	if resources == nil {
		return nil
	}
	sourceProfiles := selectorProfiles(resources.Capabilities(), resources, namespace, source)
	destinationProfiles := selectorProfiles(resources.Capabilities(), resources, namespace, destination)
	if len(sourceProfiles) == 0 || len(destinationProfiles) == 0 {
		return nil
	}
	intersection := make([]types.ProtocolProfile, 0)
	for profile := range sourceProfiles {
		if destinationProfiles[profile] {
			intersection = append(intersection, inventoryProfile(profile))
		}
	}
	return intersection
}

// selectorProfiles unions the profiles of all capability records whose
// workload selector matches the service.
func selectorProfiles(capabilities []types.CapabilityProfile, resources *ResourceState, namespace, service string) map[string]bool {
	profiles := make(map[string]bool)
	labels := serviceLabelSet(resources, namespace, service)
	for _, capability := range capabilities {
		if selectorMatchesLabels(capability.WorkloadSelector, service, labels) {
			for _, profile := range capability.Profiles {
				profiles[profile] = true
			}
		}
	}
	return profiles
}

// inventoryProfile builds a ProtocolProfile from a profile name. The security
// class is read off the name ("quantum" or "hybrid" substring, else
// classical), and the overhead scores 2.5, 1.5 and 1.0 are the fixed inputs of
// the cost term of the ranking. They are modelled parameters and not
// measurements.
func inventoryProfile(name string) types.ProtocolProfile {
	profile := types.ProtocolProfile{Name: name, OverheadScore: 2.0}
	switch {
	case strings.Contains(strings.ToLower(name), "quantum"):
		profile.SecurityCategory = types.SecurityQuantumSafe
		profile.OverheadScore = 2.5
	case strings.Contains(strings.ToLower(name), "hybrid"):
		profile.SecurityCategory = types.SecurityHybrid
		profile.OverheadScore = 1.5
	default:
		profile.SecurityCategory = types.SecurityClassical
		profile.OverheadScore = 1.0
	}
	return profile
}
