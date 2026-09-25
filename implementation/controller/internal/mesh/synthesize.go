// Assignment synthesis: from a decided edge to Istio objects (Sect. 3,
// implementation).
//
// SynthesizeAssignment maps an edge with a desired profile to two resources, a
// PeerAuthentication (peerauth.go) and a DestinationRule (destinationrule.go).
// Both enforce ordinary Istio mutual TLS. The selected QTPO profile travels as the
// annotation quantumtrustkg.io/profile, which is the field a migration changes.
// The mesh objects therefore do not negotiate a post-quantum group themselves.
// This is the boundary the paper draws between control-plane decisions and the
// cryptography on the wire (Sect. 3 and Sect. 5).
//
// The functions here are pure. They build strings and touch no cluster, which means the
// tests in tests/unit/mesh_test.go can check the output directly.

package mesh

import "quantumtrustkg/controller/pkg/types"

// Resource captures the synthesized Kubernetes manifest for one mesh object.
type Resource struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	EdgeID     string
	Manifest   string
}

// Identifier returns kind/namespace/name, the key used to detect duplicates.
func (r Resource) Identifier() string {
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// SynthesizeAssignment returns the manifests for an edge. It returns nil when
// no profile has been selected, which means a blocked edge produces no mesh change and
// its previous objects stay as they are.
func SynthesizeAssignment(edge types.CommunicationEdge) []Resource {
	if edge.DesiredProfile == "" {
		return nil
	}
	return []Resource{
		BuildPeerAuthentication(edge),
		BuildDestinationRule(edge),
	}
}
