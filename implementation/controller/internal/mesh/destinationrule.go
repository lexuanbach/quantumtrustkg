// DestinationRule synthesis (Sect. 3, implementation).
//
// BuildDestinationRule emits the client-side half of an edge's assignment: an
// ISTIO_MUTUAL TLS policy towards the destination host, with the selected profile,
// the edge ID and the trust boundary as annotations. The name is
// "<edge-id>-tls", which health.go looks up. The connection-pool limit depends
// on the required class (16 for quantum-safe, 32 for hybrid, 64 otherwise). This
// is a fixed setting of the artifact and plays no role in admissibility.

package mesh

import (
	"fmt"

	"quantumtrustkg/controller/pkg/types"
)

// BuildDestinationRule renders the manifest for one edge.
func BuildDestinationRule(edge types.CommunicationEdge) Resource {
	name := sanitizeName(edge.ID + "-tls")
	manifest := fmt.Sprintf(`apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: quantumtrustkg
  annotations:
    quantumtrustkg.io/edge-id: %s
    quantumtrustkg.io/profile: %s
    quantumtrustkg.io/trust-boundary: %s
spec:
  host: %s.%s.svc.cluster.local
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
      sni: %s.%s.svc.cluster.local
    connectionPool:
      tcp:
        maxConnections: %d
`, name, edge.Namespace, edge.ID, edge.DesiredProfile, edge.TrustBoundary, edge.DestinationService, edge.Namespace, edge.DestinationService, edge.Namespace, connectionLimit(edge.RequiredSecurityLevel))

	return Resource{
		APIVersion: "networking.istio.io/v1beta1",
		Kind:       "DestinationRule",
		Namespace:  edge.Namespace,
		Name:       name,
		EdgeID:     edge.ID,
		Manifest:   manifest,
	}
}

// connectionLimit returns the TCP connection cap for a required security class.
func connectionLimit(level types.SecurityCategory) int {
	switch level {
	case types.SecurityQuantumSafe:
		return 16
	case types.SecurityHybrid:
		return 32
	default:
		return 64
	}
}
