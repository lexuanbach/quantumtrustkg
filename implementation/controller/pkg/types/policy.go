// Package types holds the shared data model of the controller: the declared
// security classes, policies, protocol profiles, endpoint capabilities,
// provenance and the assignment record of a communication edge (Sect. 2 and
// Sect. 3 of the paper). These are the Go counterparts of the records in
// formal/coq/Model.v. The Coq model reduces most of them to Booleans, and the
// controller keeps the underlying values, for example timestamps. With them it
// can compute those Booleans and log the reasons.
package types

// SecurityCategory is a declared cryptographic class. A policy uses it as the
// minimum required class and a profile uses it as the class it offers. The
// classes are ordered classical, hybrid and quantum_safe, the order written
// classical <= hybrid <= quantum_safe in Sect. 2 (security_class in the Coq
// model). The class is declared metadata and is not verified against the
// negotiated handshake.
type SecurityCategory string

const (
	SecurityClassical   SecurityCategory = "classical"
	SecurityHybrid      SecurityCategory = "hybrid"
	SecurityQuantumSafe SecurityCategory = "quantum_safe"
)

// Policy describes an intent-level control-plane policy before it is translated
// into mesh-specific resources. RequiredSecurityCategory is the minimum class
// it demands for the edges it governs, and the strongest attached requirement
// becomes Req(e). AllowHybrid is the explicit permission that opens the guarded
// hybrid fallback. Compliance lists obligations such as PCI-DSS that can raise
// the requirement of a regulated edge. TrustBoundaryPolicy names the boundary
// the policy is bound to when the policy is attached by boundary and not by
// service.
type Policy struct {
	Name                     string
	Namespace                string
	Selector                 map[string]string
	RequiredSecurityCategory SecurityCategory
	AllowHybrid              bool
	Compliance               []string
	TrustBoundaryPolicy      string
}
