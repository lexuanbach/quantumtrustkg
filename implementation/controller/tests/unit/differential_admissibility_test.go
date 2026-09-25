// Differential test of the Go selector against the Coq model (Thm. 1 and
// Thm. 2, G1, G2 and G4).
//
// The paper states that the Go controller and the Coq/Rocq model are linked by
// differential testing, and that no machine-checked refinement exists (Sect. 3 and
// Sect. 5). This file is that test. It contains an executable transliteration of the
// predicates of formal/coq (see the comment below) and runs it against the Go code on
// seeded random cases:
//
//   TestSelectorAgreesWithRocqSpecification  5000 cases from seed 20260511.
//       The Go path (CompatibleProfiles, ValidateEdge, SelectProfile) and the
//       transliterated select must agree on promote or block. A promoted profile
//       must be a direct or fallback candidate of the model (G1 and G2), and the
//       pre-commit checker must accept it. Fail-closed cases (stale or
//       contradictory facts, missing capability record) must block (G4). The test
//       also fails if the generator misses any of the outcomes promoted,
//       fallback or blocked.
//   TestCheckPromotionAgreesWithRocqPredicates  3000 cases from seed 20260512.
//       CheckPromotion is compared with the model's direct-or-fallback predicate
//       for every profile and every combination of the six availability gates.
//
// Both tests are deterministic. A failing case index and seed reproduce the case.

package unit

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"quantumtrustkg/controller/internal/graph"
	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// The spec* helpers below are an executable transliteration of the Coq model in
// formal/coq (Model.v: endpoint_supportb, threat_okb, direct_candidateb,
// fallback_candidateb. QTPO.v: gate_failure, select). The Coq select promotes the
// first satisfying profile and Go promotes the minimum-score one. The comparison
// therefore covers the promote or block outcome and the admissibility of the
// promoted profile, and it does not cover which admissible profile is chosen.

// specState is one generated case: the endpoints' advertised sets, the candidate
// profiles, the runtime deprecations, the required class, the hybrid permission
// and the availability of each gate's facts.
type specState struct {
	srcProfiles, dstProfiles map[string]bool // nil = no service record
	profiles                 []types.ProtocolProfile
	deprecated               map[string]bool
	required                 types.SecurityCategory
	permitsHybrid            bool
	graphAvailable           bool
	factAvailable            bool
	factFresh                bool
	factConflict             bool
	rolloutFeasible          bool
	meshReady                bool
}

// specRank orders the classes like the Coq class_le: classical < hybrid < quantum-safe.
func specRank(c types.SecurityCategory) int {
	switch c {
	case types.SecurityClassical:
		return 1
	case types.SecurityHybrid:
		return 2
	case types.SecurityQuantumSafe:
		return 3
	}
	return 0
}

// specSupports mirrors supports_profileb. A missing service record supports
// nothing.
func specSupports(set map[string]bool, name string) bool { return set != nil && set[name] }

// specEndpointSupport mirrors endpoint_supportb: both endpoints advertise the profile.
func specEndpointSupport(st specState, p types.ProtocolProfile) bool {
	return specSupports(st.srcProfiles, p.Name) && specSupports(st.dstProfiles, p.Name)
}

// specThreatOK mirrors threat_okb. The model's profile_deprecated folds
// deprecation, insufficient proof status and active advisory into one test.
func specThreatOK(st specState, p types.ProtocolProfile) bool {
	deprecated := p.Deprecated || st.deprecated[p.Name] || (p.Family != "" && st.deprecated[p.Family]) ||
		p.ProofStatus == "insufficient" || p.Advisory != ""
	return !deprecated
}

// specGatesOK is the negation of gate_failure: every availability gate holds.
func specGatesOK(st specState) bool {
	return st.graphAvailable && st.factAvailable && st.factFresh && !st.factConflict && st.rolloutFeasible && st.meshReady
}

// specDirect mirrors direct_candidateb.
func specDirect(st specState, p types.ProtocolProfile) bool {
	return specEndpointSupport(st, p) && specThreatOK(st, p) && specRank(st.required) <= specRank(p.SecurityCategory) && specGatesOK(st)
}

// specFallback mirrors fallback_candidateb, which needs a robust hybrid profile
// and a policy that permits hybrid.
func specFallback(st specState, p types.ProtocolProfile) bool {
	return specEndpointSupport(st, p) && specThreatOK(st, p) && p.SecurityCategory == types.SecurityHybrid &&
		p.HybridRobust && st.permitsHybrid && specGatesOK(st)
}

// specSelect mirrors select. It checks gate_failure first, then takes the first
// direct candidate, then the first fallback candidate if policy permits, and
// otherwise blocks. It returns whether a profile is promoted and whether the
// promotion is direct.
func specSelect(st specState) (promoted bool, direct bool) {
	if !specGatesOK(st) {
		return false, false
	}
	for _, p := range st.profiles {
		if specDirect(st, p) {
			return true, true
		}
	}
	if st.permitsHybrid {
		for _, p := range st.profiles {
			if specFallback(st, p) {
				return true, false
			}
		}
	}
	return false, false
}

// randomSet draws an advertised set, or nil (no capability record) with
// probability 1/6.
func randomSet(rng *rand.Rand, names []string) map[string]bool {
	if rng.Intn(6) == 0 {
		return nil // no capability record
	}
	set := map[string]bool{}
	for _, n := range names {
		if rng.Intn(3) != 0 {
			set[n] = true
		}
	}
	return set
}

// generatedSpecCase draws one random case with four profiles (classical, two
// hybrid, quantum-safe), random deprecations, proof status and advisories, and
// stale or contradictory facts at rates of about 10% and 8%.
func generatedSpecCase(rng *rand.Rand, i int) specState {
	levels := []types.SecurityCategory{types.SecurityClassical, types.SecurityHybrid, types.SecurityQuantumSafe}
	profiles := []types.ProtocolProfile{
		{Name: fmt.Sprintf("classical-%d", i), Family: "ecdhe", SecurityCategory: types.SecurityClassical, OverheadScore: 0.8 + rng.Float64()},
		{Name: fmt.Sprintf("hybrid-%d", i), Family: "x25519mlkem", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.0 + rng.Float64(), HybridRobust: rng.Intn(4) != 0},
		{Name: fmt.Sprintf("hybrid2-%d", i), Family: "p256mlkem", SecurityCategory: types.SecurityHybrid, OverheadScore: 1.0 + rng.Float64(), HybridRobust: rng.Intn(4) != 0},
		{Name: fmt.Sprintf("quantum-%d", i), Family: "mlkem", SecurityCategory: types.SecurityQuantumSafe, OverheadScore: 1.2 + rng.Float64()},
	}
	for j := range profiles {
		switch rng.Intn(12) {
		case 0:
			profiles[j].Deprecated = true
		case 1:
			profiles[j].ProofStatus = "insufficient"
		case 2:
			profiles[j].Advisory = "ADV-generated"
		}
	}
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	deprecated := map[string]bool{}
	if rng.Intn(8) == 0 {
		deprecated["mlkem"] = true
	}
	return specState{
		srcProfiles:     randomSet(rng, names),
		dstProfiles:     randomSet(rng, names),
		profiles:        profiles,
		deprecated:      deprecated,
		required:        levels[rng.Intn(len(levels))],
		permitsHybrid:   rng.Intn(2) == 0,
		graphAvailable:  true,
		factAvailable:   true,
		factFresh:       rng.Intn(10) != 0,
		factConflict:    rng.Intn(12) == 0,
		rolloutFeasible: true,
		meshReady:       true,
	}
}

// capsFrom converts an advertised set to endpoint capabilities. A nil set is an
// unknown endpoint.
func capsFrom(set map[string]bool) types.EndpointCapabilities {
	if set == nil {
		return types.EndpointCapabilities{}
	}
	profiles := make([]string, 0, len(set))
	for n := range set {
		profiles = append(profiles, n)
	}
	return types.EndpointCapabilities{Known: true, Profiles: profiles}
}

// edgeFromSpec builds the communication edge that corresponds to a case.
func edgeFromSpec(st specState, i int, now time.Time) types.CommunicationEdge {
	freshUntil := now.Add(5 * time.Minute)
	if !st.factFresh {
		freshUntil = now.Add(-time.Minute)
	}
	conflict := ""
	if st.factConflict {
		conflict = "generated contradictory capability"
	}
	return types.CommunicationEdge{
		ID:                      fmt.Sprintf("spec-%03d", i),
		SourceService:           "src",
		DestinationService:      "dst",
		TrustBoundary:           "internal",
		RequiredSecurityLevel:   st.required,
		CandidateProfiles:       append([]types.ProtocolProfile(nil), st.profiles...),
		Policies:                []types.Policy{{Name: "generated", AllowHybrid: st.permitsHybrid}},
		SourceCapabilities:      capsFrom(st.srcProfiles),
		DestinationCapabilities: capsFrom(st.dstProfiles),
		Provenance:              types.EdgeProvenance{ObservedAt: now.Add(-time.Minute), FreshUntil: freshUntil, ConflictNote: conflict},
	}
}

// goSelect follows the order of the runtime: compute C_e, validate the metadata,
// then select.
func goSelect(edge types.CommunicationEdge, deprecated map[string]bool, now time.Time) (types.CommunicationEdge, bool) {
	compatible, _ := orchestration.CompatibleProfiles(edge)
	edge.CandidateProfiles = compatible
	if graph.ValidateEdge(edge, now) != nil {
		return edge, false
	}
	return orchestration.SelectProfile(edge, deprecated)
}

// TestSelectorAgreesWithRocqSpecification is the first differential test
// described in the file header.
func TestSelectorAgreesWithRocqSpecification(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(20260511))
	promotedCases, fallbackCases, blockedCases := 0, 0, 0
	for i := 0; i < 5000; i++ {
		st := generatedSpecCase(rng, i)
		edge := edgeFromSpec(st, i, now)
		wantPromoted, wantDirect := specSelect(st)
		selected, ok := goSelect(edge, st.deprecated, now)
		if ok != wantPromoted {
			t.Fatalf("case %d: Go promoted=%v, Rocq spec promoted=%v (reason=%s)", i, ok, wantPromoted, selected.Reason)
		}
		if !ok {
			blockedCases++
			continue
		}
		promotedCases++
		chosen, found := profileByNameT(st.profiles, selected.DesiredProfile)
		if !found {
			t.Fatalf("case %d: promoted unknown profile %s", i, selected.DesiredProfile)
		}
		if wantDirect && !specDirect(st, chosen) {
			t.Fatalf("case %d: Go promoted %s which is not a Rocq direct candidate", i, chosen.Name)
		}
		if !wantDirect {
			fallbackCases++
			if !specFallback(st, chosen) {
				t.Fatalf("case %d: Go promoted %s which is not a Rocq fallback candidate", i, chosen.Name)
			}
		}
		facts := orchestration.PromotionFacts{GraphAvailable: true, FactAvailable: true, FactFresh: st.factFresh,
			FactConflict: st.factConflict, RolloutFeasible: true, MeshReady: true}
		if good, reason := orchestration.CheckPromotion(edge, chosen, facts, st.deprecated); !good {
			t.Fatalf("case %d: pre-commit checker rejected a Rocq-admissible promotion: %s", i, reason)
		}
	}
	if promotedCases == 0 || fallbackCases == 0 || blockedCases == 0 {
		t.Fatalf("generator did not cover all outcomes: promoted=%d fallback=%d blocked=%d", promotedCases, fallbackCases, blockedCases)
	}
}

// TestCheckPromotionAgreesWithRocqPredicates is the second differential test
// described in the file header. It covers every profile and gate combination,
// including profiles the selector would never pick.
func TestCheckPromotionAgreesWithRocqPredicates(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(20260512))
	for i := 0; i < 3000; i++ {
		st := generatedSpecCase(rng, i)
		st.graphAvailable = rng.Intn(10) != 0
		st.factAvailable = rng.Intn(10) != 0
		st.rolloutFeasible = rng.Intn(10) != 0
		st.meshReady = rng.Intn(5) != 0
		edge := edgeFromSpec(st, i, now)
		facts := orchestration.PromotionFacts{GraphAvailable: st.graphAvailable, FactAvailable: st.factAvailable,
			FactFresh: st.factFresh, FactConflict: st.factConflict, RolloutFeasible: st.rolloutFeasible, MeshReady: st.meshReady}
		for _, p := range st.profiles {
			want := specDirect(st, p) || specFallback(st, p)
			got, reason := orchestration.CheckPromotion(edge, p, facts, st.deprecated)
			if got != want {
				t.Fatalf("case %d profile %s: checker=%v (%s) spec=%v", i, p.Name, got, reason, want)
			}
		}
	}
}

// profileByNameT finds a profile by name in a test candidate list.
func profileByNameT(ps []types.ProtocolProfile, name string) (types.ProtocolProfile, bool) {
	for _, p := range ps {
		if p.Name == name {
			return p, true
		}
	}
	return types.ProtocolProfile{}, false
}
