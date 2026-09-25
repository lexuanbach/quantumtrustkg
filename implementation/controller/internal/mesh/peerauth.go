// PeerAuthentication synthesis (Sect. 3, implementation).
//
// For an edge, BuildPeerAuthentication emits a STRICT mutual-TLS policy that
// selects the destination workload by its app label. The selected profile and the
// required security level are recorded as annotations (quantumtrustkg.io/profile
// and quantumtrustkg.io/security-level). The name is "<edge-id>-mtls", which is
// the name ClusterHealthObserver looks up when it checks that the object exists
// (health.go).

package mesh

import (
	"fmt"
	"strings"

	"quantumtrustkg/controller/pkg/types"
)

// BuildPeerAuthentication renders the manifest for one edge. The mode is always
// STRICT. The profile is metadata only.
func BuildPeerAuthentication(edge types.CommunicationEdge) Resource {
	name := sanitizeName(edge.ID + "-mtls")
	manifest := fmt.Sprintf(`apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: quantumtrustkg
  annotations:
    quantumtrustkg.io/edge-id: %s
    quantumtrustkg.io/profile: %s
    quantumtrustkg.io/security-level: %s
spec:
  selector:
    matchLabels:
      app: %s
  mtls:
    mode: STRICT
`, name, edge.Namespace, edge.ID, edge.DesiredProfile, edge.RequiredSecurityLevel, edge.DestinationService)

	return Resource{
		APIVersion: "security.istio.io/v1beta1",
		Kind:       "PeerAuthentication",
		Namespace:  edge.Namespace,
		Name:       name,
		EdgeID:     edge.ID,
		Manifest:   manifest,
	}
}

// sanitizeName maps characters that are not allowed in a Kubernetes object name
// or a file name to "-". It does not lower-case, which means callers that need lower case
// apply strings.ToLower first.
func sanitizeName(in string) string {
	replacer := strings.NewReplacer("_", "-", " ", "-", "/", "-", ":", "-")
	return replacer.Replace(in)
}
