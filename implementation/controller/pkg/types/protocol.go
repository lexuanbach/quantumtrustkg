package types

// ProtocolProfile describes a deployable cryptographic profile that QTPO can
// assign to an edge (the set P of Sect. 2).
//
// Besides the declared security class and the overhead used for ranking, a
// profile carries the threat metadata that hard gates (ii) and (iii) read.
// That metadata is the deprecation of the primitive family, the proof or
// standardization status and any active advisory. For hybrid profiles it also
// records whether the combiner is robust, meaning that it stays secure while at
// least one component assumption holds. The hybrid fallback admits only
// profiles with HybridRobust set, as profile_hybrid_robust does in the Coq
// model. All fields are declared metadata that the controller does not verify
// on the wire. KeyExchange and Authentication are descriptive labels, and no
// gate or score reads them.
type ProtocolProfile struct {
	Name             string
	KeyExchange      string
	Authentication   string
	SecurityCategory SecurityCategory
	OverheadScore    float64

	// Family names the primitive family, for example ML-KEM or X25519MLKEM768.
	// A deprecation recorded against the family applies to every profile of
	// that family.
	Family string
	// Deprecated marks the profile or its family as deprecated.
	Deprecated bool
	// ProofStatus records the standardization or proof status. The values
	// insufficient, withdrawn and broken fail gate (ii). An empty value means
	// that no negative status has been recorded.
	ProofStatus string
	// Advisory names an active threat advisory that targets the profile. Any
	// non-empty value fails gate (iii).
	Advisory string
	// AdvisorySeverity in [0,1] is the largest severity among non-blocking,
	// informational advisories. It enters the Risk term of the score and is
	// never read by an admissibility check.
	AdvisorySeverity float64
	// HybridRobust records that a hybrid combiner is modelled as secure while at
	// least one component assumption remains hard.
	HybridRobust bool
}
