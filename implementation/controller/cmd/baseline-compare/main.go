// Command baseline-compare is the cluster-free selection harness (H2) of the
// evaluation, Sect. 4. It produces the data behind Table 1 (RQ1), the real-graph
// curves of Fig. 2(a) (RQ2), the Online Boutique ablation row and the weight and
// hysteresis sweep of Sect. 5.
//
// The harness isolates the selection logic. It generates a seeded population of
// communication edges, runs QTPO and each competing selector on the identical
// population, and classifies every edge as one of three outcomes:
//
//	compliant        promoted at or above the required class Req(e), or on a
//	                 hybrid fallback that the policy permits
//	blocked-safe     not promoted, which means the previous assignment stays
//	unsafe           promoted below Req(e), or promoted on stale or contradictory
//	                 provenance (a corrupted fact)
//
// The unsafe rate is the paper's safety metric and is independent of whether the
// declared labels are truthful. The compliant share is the declared-label policy
// compliance, which reads the same labels that drive the selector. It therefore
// scores control-plane decisions and says nothing about the cryptography that is
// negotiated on the wire (Sect. 4, Metrics, and Sect. 5).
//
// Mesh generation (generateMesh). Each service gets a capability tier (8% classical
// only, 37% hybrid, 55% quantum-safe) and receives one to three inbound edges.
// About 40% of quantum-safe-capable destinations get a hybrid-capped partner edge
// next to their regulated edges, which is the requirement divergence that a single
// per-service posture cannot satisfy. About 3% of edges carry corrupted provenance
// (half stale, half contradictory). Sub-seeds are seed + 7919*rep + n, which means every run
// is reproducible from the stated seed (20260509 in the paper).
//
// Selectors. QTPO and QTPO-NoKG call the real orchestration.SelectProfile, and
// NoKG omits the freshness and contradiction gate. The competing systems are
// re-creations of one selection rule each (Sect. 4, Baseline implementation):
//
//	SL-CA          one posture per destination service, the strongest class that
//	               every inbound edge supports
//	ELCA           a central per-service mandate equal to the strongest class any
//	               edge requires, applied uniformly, with no provenance gating
//	ConfigProfile  a per-endpoint configuration with provenance tracking, which means it
//	               fails closed on bad facts but cannot express per-edge divergence
//	OPP            the strongest mutually supported class per edge, trusting claims
//	Static         a fixed mesh-wide hybrid posture
//
// evalOPA and evalGreedy implement two further baselines (baselines/opa-guided.yaml
// and baselines/greedy-non-kg.yaml). They are not part of the method list in main
// and do not appear in Table 1.
//
// Scenarios (flag --scenario) and the result file each one writes:
//
//	synthetic        Table 1 at 50/100/200 services   baseline-comparison.csv
//	online-boutique  11-edge Online Boutique ablation online-boutique-ablation.csv
//	real-topology    sampled Alibaba subgraphs, Fig. 2(a)  baseline-comparison-real-alibaba.csv
//	weight-sweep     +-50% weights and delta          weight-sweep-results.csv
//
// In the real-topology scenario only the dependency topology is real. The trace
// carries no cryptographic policy, which means tiers, boundaries and requirements come from
// the same calibrated model as the synthetic mesh.
//
// The harness needs no Fuseki, Kubernetes or Istio. Latency, setup cost and scale
// figures come from the other commands and scripts.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"quantumtrustkg/controller/internal/orchestration"
	"quantumtrustkg/controller/pkg/types"
)

// rank maps a security class to an integer: 0 classical, 1 hybrid, 2 quantum-safe.
func rank(c types.SecurityCategory) int {
	switch c {
	case types.SecurityQuantumSafe:
		return 2
	case types.SecurityHybrid:
		return 1
	default:
		return 0
	}
}

// classForRank is the inverse of rank.
func classForRank(r int) types.SecurityCategory {
	switch {
	case r >= 2:
		return types.SecurityQuantumSafe
	case r == 1:
		return types.SecurityHybrid
	default:
		return types.SecurityClassical
	}
}

// genEdge is one generated communication edge together with the ground truth
// the evaluators need. dstService lets the service-level baselines group the
// inbound edges of a service.
type genEdge struct {
	edge        types.CommunicationEdge
	deprecated  map[string]bool
	dstService  string
	support     int  // strongest class mutually supported by both endpoints
	provBad     bool // stale or contradictory provenance, which must block and never promote
	req         int  // required class rank
	allowHybrid bool
}

// profilesUpTo returns one profile per class up to tier, which means an edge whose
// endpoints support at most hybrid has no quantum-safe candidate. idx makes the
// profile names unique per edge. The overhead scores 0.8, 1.1 and 1.4 are model
// inputs of the cost ranking.
func profilesUpTo(tier, idx int) []types.ProtocolProfile {
	all := []types.ProtocolProfile{
		{Name: fmt.Sprintf("classical-%d", idx), SecurityCategory: types.SecurityClassical, OverheadScore: 0.8},
		{Name: fmt.Sprintf("hybrid-%d", idx), SecurityCategory: types.SecurityHybrid, OverheadScore: 1.1, HybridRobust: true},
		{Name: fmt.Sprintf("quantum-%d", idx), SecurityCategory: types.SecurityQuantumSafe, OverheadScore: 1.4},
	}
	out := make([]types.ProtocolProfile, 0, tier+1)
	for _, p := range all {
		if rank(p.SecurityCategory) <= tier {
			out = append(out, p)
		}
	}
	return out
}

// generateMesh builds a deterministic population of about 2n edges for a mesh of
// n services. Roughly 70% of edges have a clear admissible choice. The remaining
// 30% carry the edge-level divergence that motivates the paper. These are
// regulated boundaries that need quantum-safe protection and share a service with
// partner or transitional edges, plus a small fraction of stale or contradictory
// provenance.
func generateMesh(n int, seed int64) []genEdge {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	// Each service gets a capability tier. The split models a mesh in the middle
	// of a migration. Most services are hybrid or quantum-safe capable, with a
	// small tail of legacy services that stay classical.
	tier := make([]int, n)
	for i := range tier {
		r := rng.Float64()
		switch {
		case r < 0.08:
			tier[i] = 0 // legacy / classical-only
		case r < 0.45:
			tier[i] = 1 // hybrid-capable
		default:
			tier[i] = 2 // quantum-safe-capable
		}
	}

	// pickSource returns a source of at most maxTier, which lets us attach
	// hybrid-capped partner edges to quantum-safe destinations. That divergence
	// is what defeats a service-wide posture. After eight failed draws it falls
	// back to any other service.
	pickSource := func(dst, maxTier int) int {
		for tries := 0; tries < 8; tries++ {
			s := rng.Intn(n)
			if s != dst && tier[s] <= maxTier {
				return s
			}
		}
		s := rng.Intn(n)
		if s == dst {
			s = (s + 1) % n
		}
		return s
	}

	// Every service receives one to three inbound edges, which gives about 2n
	// edges in total and mirrors the edge-to-service ratio of the testbed.
	edges := make([]genEdge, 0, 2*n)
	idx := 0
	for dst := 0; dst < n; dst++ {
		fanIn := 1 + rng.Intn(3) // 1..3 inbound edges per service
		// A quantum-safe-capable destination gets a partner edge from a
		// hybrid-capped peer with probability 0.4. No single service-wide posture
		// can then serve both its regulated and its partner edges.
		forcePartner := tier[dst] == 2 && rng.Float64() < 0.40
		for k := 0; k < fanIn; k++ {
			idx++

			var src int
			var boundary string
			var req int

			isPartnerEdge := forcePartner && k == 0
			switch {
			case isPartnerEdge:
				// Hybrid-capped partner peer. The requirement is achievable at hybrid.
				src = pickSource(dst, 1)
				boundary, req = "partner", 1
			default:
				src = pickSource(dst, 2)
				srcT, dstT := tier[src], tier[dst]
				sup := srcT
				if dstT < sup {
					sup = dstT
				}
				switch {
				case sup == 2 && rng.Float64() < 0.45:
					// Regulated edge. Both endpoints are quantum-safe capable, which means the
					// requirement is achievable per edge and QTPO can promote it.
					boundary, req = "regulated", 2
				case sup >= 1 && rng.Float64() < 0.5:
					boundary, req = "internal", 1
				default:
					boundary, req = "internal", 0
				}
			}

			srcTier, dstTier := tier[src], tier[dst]
			support := srcTier
			if dstTier < support {
				support = dstTier // mutually supported tier
			}
			// Some edges require more than their endpoints support because of a
			// legacy peer. Half of them are raised to achievable and the rest are
			// unpromotable, which means a provenance-aware controller must block them.
			if support < req && rng.Float64() < 0.5 {
				support = req // achievable after all
			}
			candidates := profilesUpTo(support, idx)

			allowHybrid := rng.Float64() < 0.6

			// About 3% of edges carry stale or contradictory provenance.
			provBad := rng.Float64() < 0.03
			freshUntil := now.Add(5 * time.Minute)
			conflict := ""
			if provBad {
				if rng.Intn(2) == 0 {
					freshUntil = now.Add(-time.Minute) // stale
				} else {
					conflict = "generated contradictory capability"
				}
			}

			e := types.CommunicationEdge{
				ID:                    fmt.Sprintf("edge-%05d", idx),
				TrustBoundary:         boundary,
				RequiredSecurityLevel: classForRank(req),
				CandidateProfiles:     candidates,
				Policies: []types.Policy{{
					Name:        "transition",
					AllowHybrid: allowHybrid,
				}},
				Provenance: types.EdgeProvenance{
					Source:       "synthetic",
					ObservedAt:   now.Add(-time.Minute),
					FreshUntil:   freshUntil,
					ConflictNote: conflict,
				},
			}
			deprecated := map[string]bool{}

			_ = srcTier
			_ = dstTier
			edges = append(edges, genEdge{
				edge:        e,
				deprecated:  deprecated,
				dstService:  fmt.Sprintf("svc-%04d", dst),
				support:     support,
				provBad:     provBad,
				req:         req,
				allowHybrid: allowHybrid,
			})
		}
	}
	return edges
}

// meshFromTopology builds a mesh over a real service-dependency topology, given
// as index pairs from a production trace or benchmark. The topology is real,
// including the fan-in, fan-out and hub structure. Production traces carry no
// cryptographic policy, which means the per-edge requirements come from the same
// calibrated model as the synthetic mesh, with 30% of services marked regulated.
// Requirement divergence then arises from the real fan-in. A regulated
// destination with mixed-tier callers receives both a quantum-safe regulated edge
// and a hybrid-capped partner edge. This is the RQ2 setting of Sect. 4.2.
func meshFromTopology(topo [][2]int, seed int64) []genEdge {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	n := 0
	for _, e := range topo {
		if e[0]+1 > n {
			n = e[0] + 1
		}
		if e[1]+1 > n {
			n = e[1] + 1
		}
	}
	tier := make([]int, n)
	regulated := make([]bool, n)
	for i := range tier {
		switch r := rng.Float64(); {
		case r < 0.08:
			tier[i] = 0
		case r < 0.45:
			tier[i] = 1
		default:
			tier[i] = 2
		}
		regulated[i] = rng.Float64() < 0.30
	}
	edges := make([]genEdge, 0, len(topo))
	for idx, e := range topo {
		src, dst := e[0], e[1]
		support := tier[src]
		if tier[dst] < support {
			support = tier[dst]
		}
		var req int
		var boundary string
		switch {
		case regulated[dst] && support == 2:
			boundary, req = "regulated", 2
		case regulated[dst]:
			boundary, req = "partner", 1 // hybrid-capped edge into a regulated service
		case support >= 1 && rng.Float64() < 0.5:
			boundary, req = "internal", 1
		default:
			boundary, req = "internal", 0
		}
		if support < req && rng.Float64() < 0.5 {
			support = req
		}
		candidates := profilesUpTo(support, idx)
		allowHybrid := rng.Float64() < 0.6
		provBad := rng.Float64() < 0.03
		freshUntil := now.Add(5 * time.Minute)
		conflict := ""
		if provBad {
			if rng.Intn(2) == 0 {
				freshUntil = now.Add(-time.Minute)
			} else {
				conflict = "generated contradictory capability"
			}
		}
		ce := types.CommunicationEdge{
			ID:                    fmt.Sprintf("edge-%05d", idx),
			TrustBoundary:         boundary,
			RequiredSecurityLevel: classForRank(req),
			CandidateProfiles:     candidates,
			Policies:              []types.Policy{{Name: "transition", AllowHybrid: allowHybrid}},
			Provenance: types.EdgeProvenance{
				Source:       "real-topology",
				ObservedAt:   now.Add(-time.Minute),
				FreshUntil:   freshUntil,
				ConflictNote: conflict,
			},
		}
		edges = append(edges, genEdge{
			edge:        ce,
			deprecated:  map[string]bool{},
			dstService:  fmt.Sprintf("svc-%d", dst),
			support:     support,
			provBad:     provBad,
			req:         req,
			allowHybrid: allowHybrid,
		})
	}
	return edges
}

// loadTopologies reads the *.edges files in dir. Each line holds a whitespace
// separated "src dst" pair of service names. Names are remapped to dense indices
// and the result maps the service count to the edge list, which means each file is one
// scale of Fig. 2(a). Files whose name contains "full" are skipped because they
// hold the unsampled parent graph.
func loadTopologies(dir string) map[int][][2]int {
	out := map[int][][2]int{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.edges"))
	for _, fn := range files {
		if strings.Contains(filepath.Base(fn), "full") {
			continue
		}
		data, err := os.ReadFile(fn)
		if err != nil {
			continue
		}
		idxOf := map[string]int{}
		var topo [][2]int
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) != 2 {
				continue
			}
			for _, name := range f {
				if _, ok := idxOf[name]; !ok {
					idxOf[name] = len(idxOf)
				}
			}
			topo = append(topo, [2]int{idxOf[f[0]], idxOf[f[1]]})
		}
		if len(idxOf) > 0 {
			out[len(idxOf)] = topo
		}
	}
	return out
}

// generateOnlineBoutique builds the 11 managed edges of the Online Boutique
// fixture, the microservices demo of Google Cloud Platform, with their trust
// boundaries. Checkout to Payment is regulated and needs quantum-safe protection,
// Checkout to Shipping is a hybrid-capped partner edge, and the rest are
// internal. Each repetition corrupts the provenance of an edge with probability
// 0.06. QTPO then blocks the edge, while a provenance-blind method promotes it.
func generateOnlineBoutique(seed int64) []genEdge {
	rng := rand.New(rand.NewSource(seed))
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	type spec struct {
		dst      string
		boundary string
		req      int // required class rank
		support  int // mutually supported tier
	}
	specs := []spec{
		{"checkout", "internal", 1, 2},
		{"productcatalog", "internal", 0, 2},
		{"cart", "internal", 1, 2},
		{"currency", "internal", 0, 2},
		{"payment", "regulated", 2, 2}, // PCI obligation: quantum-safe required, endpoints capable
		{"cart2", "internal", 1, 2},    // checkout to cart
		{"shipping", "partner", 1, 1},  // hybrid-capped partner peer
		{"email", "internal", 0, 2},    // checkout to email
		{"redis", "internal", 1, 2},    // cart to redis
		{"productcatalog2", "internal", 0, 2},
		{"ad", "internal", 0, 2},
	}
	edges := make([]genEdge, 0, len(specs))
	for i, sp := range specs {
		allowHybrid := true
		provBad := rng.Float64() < 0.06
		freshUntil := now.Add(5 * time.Minute)
		conflict := ""
		if provBad {
			if rng.Intn(2) == 0 {
				freshUntil = now.Add(-time.Minute)
			} else {
				conflict = "generated contradictory capability"
			}
		}
		e := types.CommunicationEdge{
			ID:                    fmt.Sprintf("ob-%02d", i),
			TrustBoundary:         sp.boundary,
			RequiredSecurityLevel: classForRank(sp.req),
			CandidateProfiles:     profilesUpTo(sp.support, i),
			Policies:              []types.Policy{{Name: "ob", AllowHybrid: allowHybrid}},
			Provenance: types.EdgeProvenance{
				Source: "online-boutique", ObservedAt: now.Add(-time.Minute),
				FreshUntil: freshUntil, ConflictNote: conflict,
			},
		}
		edges = append(edges, genEdge{
			edge: e, deprecated: map[string]bool{}, dstService: sp.dst,
			support: sp.support, provBad: provBad, req: sp.req, allowHybrid: allowHybrid,
		})
	}
	return edges
}

// outcome counts how a method handled the edges of one mesh.
type outcome struct {
	compliant       int // promoted with class >= req, or on a policy-permitted hybrid fallback
	blockedSafe     int // blocked, which is safe because the previous assignment stays
	unsafePromotion int // promoted with class < req, or promoted on bad provenance
	managed         int
}

// compliancePct is the declared-label compliance in percent of managed edges. It
// is the correct_promotion_pct column of the CSV files.
func (o outcome) compliancePct() float64 {
	if o.managed == 0 {
		return 0
	}
	return 100 * float64(o.compliant) / float64(o.managed)
}

// metadataValid mirrors the controller's pre-selection validation, which is the
// freshness and contradiction check of gate (iv). A provenance-aware method must
// not promote an edge that fails it.
func metadataValid(g genEdge, now time.Time) bool {
	if g.edge.Provenance.FreshUntil.IsZero() || now.After(g.edge.Provenance.FreshUntil) {
		return false
	}
	if g.edge.Provenance.ConflictNote != "" {
		return false
	}
	return true
}

// evalQTPO runs the controller's own orchestration.SelectProfile behind the
// provenance check, which is the full QTPO procedure of Alg. 1.
func evalQTPO(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		if !metadataValid(g, now) {
			o.blockedSafe++ // fail closed on stale or contradictory facts
			continue
		}
		sel, ok := orchestration.SelectProfile(g.edge, g.deprecated)
		if !ok {
			o.blockedSafe++
			continue
		}
		chosen, found := profileByName(g.edge.CandidateProfiles, sel.DesiredProfile)
		if !found {
			o.blockedSafe++
			continue
		}
		if classifyPromotion(chosen, g, sel.Reason == "policy-permitted-hybrid-fallback") {
			o.compliant++
		} else {
			o.unsafePromotion++
		}
	}
	return o
}

// evalNoKG is the QTPO-NoKG ablation. It uses QTPO's filters, ranking and
// hysteresis, but drops the graph-backed freshness and contradiction gate, which means it
// cannot fail closed on bad provenance. Policy and threat filtering stay. Its
// gap to QTPO therefore isolates the value of the provenance-carrying graph.
func evalNoKG(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		// The ablation does not consult provenance freshness or contradiction.
		sel, ok := orchestration.SelectProfile(g.edge, g.deprecated)
		if !ok {
			o.blockedSafe++
			continue
		}
		chosen, found := profileByName(g.edge.CandidateProfiles, sel.DesiredProfile)
		if !found {
			o.blockedSafe++
			continue
		}
		// The class requirement holds because the filters are the same. A
		// promotion that rests on stale or contradictory facts is still unsafe,
		// and this variant has no way to detect it.
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		if classifyPromotion(chosen, g, sel.Reason == "policy-permitted-hybrid-fallback") {
			o.compliant++
		} else {
			o.unsafePromotion++
		}
	}
	return o
}

// evalServiceLevel models SL-CA. It assigns one posture per destination service,
// the strongest class supported on every inbound edge so that no peer breaks.
// It cannot raise an individual regulated edge above that service-wide posture,
// which leaves divergent edges under-protected.
func evalServiceLevel(edges []genEdge, now time.Time) outcome {
	// The per-service ceiling is the minimum over the service's inbound edges of
	// the mutually supported tier.
	ceiling := map[string]int{}
	for _, g := range edges {
		support := g.support
		if cur, ok := ceiling[g.dstService]; !ok || support < cur {
			ceiling[g.dstService] = support
		}
	}
	var o outcome
	for _, g := range edges {
		o.managed++
		assigned := ceiling[g.dstService] // service-wide class
		// Service-level governance ignores per-edge provenance. On bad facts it
		// still promotes the static posture, which counts as unsafe.
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		if assigned >= g.req {
			o.compliant++
		} else {
			// A regulated edge is under-protected by the service-wide posture.
			o.unsafePromotion++
		}
	}
	return o
}

// evalOpportunistic models OPP. It negotiates the strongest mutually supported
// profile per edge and ignores provenance and policy-deny gating.
func evalOpportunistic(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		support := g.support
		// Opportunistic negotiation trusts capability claims at face value and
		// promotes even when provenance is stale or contradictory.
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		if support >= g.req {
			o.compliant++
		} else {
			// The required class is out of reach. OPP still negotiates the best
			// available profile and promotes it, which is an unsafe downgrade.
			o.unsafePromotion++
		}
	}
	return o
}

// evalStatic models a fixed mesh-wide hybrid posture, the Static baseline.
func evalStatic(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		assigned := 1 // hybrid
		if assigned >= g.req {
			o.compliant++
		} else {
			o.unsafePromotion++ // regulated edges are under-protected
		}
	}
	return o
}

// evalOPA models policy-as-code enforcement. It knows the required class but has
// no edge-local capability and provenance join. It asserts the required class
// where the endpoints support it, blocks where they do not, and trusts stale
// facts. It is not part of the method list in main.
func evalOPA(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		support := g.support
		if g.provBad {
			o.unsafePromotion++ // no provenance gating
			continue
		}
		if support >= g.req {
			o.compliant++
		} else {
			o.blockedSafe++ // policy can refuse, but cannot find a feasible profile
		}
	}
	return o
}

// evalGreedy models a greedy selector without the graph. It takes the
// lowest-overhead supported profile that meets the class, has no provenance
// gating and handles divergence poorly. It is not part of the method list in
// main.
func evalGreedy(edges []genEdge, now time.Time) outcome {
	var o outcome
	for _, g := range edges {
		o.managed++
		support := g.support
		if g.provBad {
			o.unsafePromotion++
			continue
		}
		if support >= g.req {
			o.compliant++
		} else if g.allowHybrid && support >= 1 && g.req <= 2 {
			// The greedy selector takes a hybrid fallback even when it is not the
			// strongest choice, and this sometimes lands below the required class.
			if 1 >= g.req {
				o.compliant++
			} else {
				o.unsafePromotion++
			}
		} else {
			o.blockedSafe++
		}
	}
	return o
}

// evalELCA models Enterprise-Level Crypto-Agility (Sikeridis et al., ePrint
// 2023/1539). ELCA offloads cryptographic operations to a central crypto service
// that is driven by a centralized per-service policy, with monitoring and
// auditing but no per-edge capability and provenance join. We model the central
// policy as mandating, per service, the strongest class any of its edges
// requires, applied uniformly. The mandate then exceeds what a hybrid-capped
// partner edge can support. Without per-edge provenance gating it also promotes
// on stale or contradictory facts.
func evalELCA(edges []genEdge, now time.Time) outcome {
	mandate := map[string]int{}
	for _, g := range edges {
		if cur, ok := mandate[g.dstService]; !ok || g.req > cur {
			mandate[g.dstService] = g.req
		}
	}
	var o outcome
	for _, g := range edges {
		o.managed++
		if g.provBad {
			o.unsafePromotion++ // no per-edge provenance gating
			continue
		}
		if mandate[g.dstService] <= g.support {
			o.compliant++ // the central mandate is feasible and at least req
		} else {
			// A uniform per-service mandate cannot be honoured on a hybrid-capped
			// partner edge whose support is below it. This is an unsafe action.
			o.unsafePromotion++
		}
	}
	return o
}

// evalConfigProfile models post-quantum TLS configuration profiling (Balaji et
// al., arXiv:2605.17955). It keeps a unified crypto inventory with provenance
// tracking and deploys one posture per endpoint configuration. It does not deploy per edge.
// Because it tracks provenance it fails closed on stale or contradictory facts,
// unlike ELCA and OPP. Its per-endpoint granularity cannot express edge-level
// divergence, though. A single endpoint configuration under-protects a regulated
// edge that shares the endpoint with a hybrid-capped partner edge.
func evalConfigProfile(edges []genEdge, now time.Time) outcome {
	// Per-endpoint ceiling: the strongest class deployable without breaking a peer.
	ceiling := map[string]int{}
	for _, g := range edges {
		if cur, ok := ceiling[g.dstService]; !ok || g.support < cur {
			ceiling[g.dstService] = g.support
		}
	}
	var o outcome
	for _, g := range edges {
		o.managed++
		if !metadataValid(g, now) {
			o.blockedSafe++ // the provenance-aware inventory fails closed on bad facts
			continue
		}
		if ceiling[g.dstService] >= g.req {
			o.compliant++
		} else {
			o.unsafePromotion++ // the endpoint configuration under-protects a divergent regulated edge
		}
	}
	return o
}

// classifyPromotion says whether a chosen profile counts as compliant. It is
// compliant when its class reaches Req(e), or when it is a hybrid profile
// reached through a fallback that the edge's policy permits. This is the same
// rule as the compliance definition of Sect. 4.
func classifyPromotion(chosen types.ProtocolProfile, g genEdge, permittedFallback bool) bool {
	if rank(chosen.SecurityCategory) >= g.req {
		return true
	}
	// A hybrid fallback is compliant only when the policy permits it.
	return permittedFallback && chosen.SecurityCategory == types.SecurityHybrid && g.allowHybrid
}

// profileByName finds a profile in a candidate list.
func profileByName(ps []types.ProtocolProfile, name string) (types.ProtocolProfile, bool) {
	for _, p := range ps {
		if p.Name == name {
			return p, true
		}
	}
	return types.ProtocolProfile{}, false
}

// runOnlineBoutique runs QTPO and the NoKG ablation on the Online Boutique edge
// set and writes online-boutique-ablation.csv. The correct_promotion_pct column
// is the median over repetitions. The unsafe rate is pooled over all
// repetitions. Each repetition uses the sub-seed seed + 7919*rep. These
// comparison scenarios start without an active assignment, so every declined
// decision is counted as no_promotion and retained_active is zero; runtime tests
// cover the separate retained-active transition.
func runOnlineBoutique(out string, seed int64, reps int, now time.Time) {
	methods := []struct {
		key string
		fn  func([]genEdge, time.Time) outcome
	}{{"QTKG", evalQTPO}, {"NoKG", evalNoKG}}
	rows := [][]string{{"fixture", "method", "correct_promotion_pct", "unsafe_promotion_pct", "unsafe_promotions", "blocked_safe", "managed_edges", "reps", "valid_promotions", "invalid_promotions", "no_promotion", "retained_active"}}
	for _, m := range methods {
		var comp []float64
		var valid, unsafe, blocked, managed int
		for r := 0; r < reps; r++ {
			edges := generateOnlineBoutique(seed + int64(r)*7919)
			o := m.fn(edges, now)
			comp = append(comp, o.compliancePct())
			unsafe += o.unsafePromotion
			blocked += o.blockedSafe
			managed += o.managed
			valid += o.compliant
		}
		sort.Float64s(comp)
		med := comp[len(comp)/2]
		rows = append(rows, []string{
			"online-boutique", m.key,
			strconv.FormatFloat(med, 'f', 2, 64),
			strconv.FormatFloat(100*float64(unsafe)/float64(managed), 'f', 2, 64),
			strconv.Itoa(unsafe), strconv.Itoa(blocked), strconv.Itoa(managed), strconv.Itoa(reps),
			strconv.Itoa(valid), strconv.Itoa(unsafe), strconv.Itoa(blocked), "0",
		})
	}
	writeRows(out, rows)
}

// runWeightSweep perturbs each QTPO weight and the hysteresis margin by +-50%
// around the operating point (orchestration.DefaultScoreConfig), one at a time.
// It also runs delta = 0. It uses the same 200-service synthetic meshes as the H2
// comparison. Half of the edges, chosen by a seeded draw, carry an existing
// active profile from their candidate list, which means the migration-cost term, the
// switch term and the hysteresis margin all have an effect. For every
// configuration it reports declared-label compliance, unsafe promotions, the
// share of promoted edges whose profile differs from the operating point's
// choice, and the share of edges with an active profile that keep it. It backs
// the sweep of Sect. 5. By Prop. 1 the weights cannot change admissibility, which means
// unsafe promotions should stay at zero in every row.
// Output: weight-sweep-results.csv.
func runWeightSweep(out string, seed int64, reps, n int, now time.Time) {
	base := orchestration.DefaultScoreConfig
	type cfgRow struct {
		name string
		cfg  orchestration.ScoreConfig
	}
	configs := []cfgRow{{"operating-point", base}}
	scale := func(label string, factor float64, set func(*orchestration.ScoreConfig, float64)) {
		c := base
		set(&c, factor)
		configs = append(configs, cfgRow{label, c})
	}
	for _, f := range []struct {
		label  string
		factor float64
	}{{"-50%", 0.5}, {"+50%", 1.5}} {
		scale("w_o"+f.label, f.factor, func(c *orchestration.ScoreConfig, k float64) { c.Wo *= k })
		scale("w_r"+f.label, f.factor, func(c *orchestration.ScoreConfig, k float64) { c.Wr *= k })
		scale("w_m"+f.label, f.factor, func(c *orchestration.ScoreConfig, k float64) { c.Wm *= k })
		scale("w_h"+f.label, f.factor, func(c *orchestration.ScoreConfig, k float64) { c.Wh *= k })
		scale("delta"+f.label, f.factor, func(c *orchestration.ScoreConfig, k float64) { c.Delta *= k })
	}
	configs = append(configs, cfgRow{"delta=0", orchestration.ScoreConfig{Wo: base.Wo, Wr: base.Wr, Wm: base.Wm, Wh: base.Wh, Delta: 0}})

	type acc struct {
		comp                                []float64
		unsafe, blocked, managed            int
		promoted, changed, withActive, kept int
	}
	results := make([]acc, len(configs))
	for r := 0; r < reps; r++ {
		edges := generateMesh(n, seed+int64(r)*7919+int64(n))
		arng := rand.New(rand.NewSource(seed + int64(r)*104729 + int64(n)))
		for i := range edges {
			if c := edges[i].edge.CandidateProfiles; len(c) > 0 && arng.Float64() < 0.5 {
				edges[i].edge.ActiveProfile = c[arng.Intn(len(c))].Name
			}
		}
		reference := make([]string, len(edges))
		for ci, c := range configs {
			var o outcome
			a := &results[ci]
			for i, g := range edges {
				o.managed++
				if !metadataValid(g, now) {
					o.blockedSafe++
					continue
				}
				sel, ok := orchestration.SelectProfileWith(g.edge, g.deprecated, c.cfg)
				if !ok {
					o.blockedSafe++
					continue
				}
				chosen, _ := profileByName(g.edge.CandidateProfiles, sel.DesiredProfile)
				if classifyPromotion(chosen, g, sel.Reason == "policy-permitted-hybrid-fallback") {
					o.compliant++
				} else {
					o.unsafePromotion++
				}
				a.promoted++
				if ci == 0 {
					reference[i] = sel.DesiredProfile
				} else if sel.DesiredProfile != reference[i] {
					a.changed++
				}
				if g.edge.ActiveProfile != "" {
					a.withActive++
					if sel.DesiredProfile == g.edge.ActiveProfile {
						a.kept++
					}
				}
			}
			a.comp = append(a.comp, o.compliancePct())
			a.unsafe += o.unsafePromotion
			a.blocked += o.blockedSafe
			a.managed += o.managed
		}
	}
	f2 := func(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }
	f3 := func(x float64) string { return strconv.FormatFloat(x, 'f', 3, 64) }
	rows := [][]string{{"config", "w_o", "w_r", "w_m", "w_h", "delta", "correct_promotion_pct", "unsafe_promotion_pct", "unsafe_promotions", "blocked_safe", "managed_edges", "selection_changed_pct", "active_retained_pct", "reps"}}
	for ci, c := range configs {
		a := results[ci]
		sort.Float64s(a.comp)
		med := a.comp[len(a.comp)/2]
		changed := 0.0
		if a.promoted > 0 {
			changed = 100 * float64(a.changed) / float64(a.promoted)
		}
		kept := 0.0
		if a.withActive > 0 {
			kept = 100 * float64(a.kept) / float64(a.withActive)
		}
		rows = append(rows, []string{
			c.name, f3(c.cfg.Wo), f3(c.cfg.Wr), f3(c.cfg.Wm), f3(c.cfg.Wh), f3(c.cfg.Delta),
			f2(med), f2(100 * float64(a.unsafe) / float64(a.managed)), strconv.Itoa(a.unsafe),
			strconv.Itoa(a.blocked), strconv.Itoa(a.managed), f2(changed), f2(kept), strconv.Itoa(reps),
		})
	}
	writeRows(out, rows)
}

// writeRows writes a CSV table to out, or to stdout when out is empty.
func writeRows(out string, rows [][]string) {
	var w *csv.Writer
	if out == "" {
		w = csv.NewWriter(os.Stdout)
	} else {
		f, err := os.Create(out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		w = csv.NewWriter(f)
	}
	if err := w.WriteAll(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	w.Flush()
}

func main() {
	out := flag.String("out", "", "CSV output path (default stdout)")
	seed := flag.Int64("seed", 20260509, "deterministic seed")
	reps := flag.Int("reps", 30, "repetitions per scale (distinct sub-seeds)")
	scenario := flag.String("scenario", "synthetic", "synthetic | online-boutique | real-topology | weight-sweep")
	topodir := flag.String("topodir", "", "directory of real *.edges topology files (real-topology scenario)")
	flag.Parse()

	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	if *scenario == "online-boutique" {
		runOnlineBoutique(*out, *seed, *reps, now)
		return
	}
	if *scenario == "weight-sweep" {
		runWeightSweep(*out, *seed, *reps, 200, now)
		return
	}
	scales := []int{50, 100, 200}
	methods := []struct {
		key string
		fn  func([]genEdge, time.Time) outcome
	}{
		{"QTKG", evalQTPO},
		{"NoKG", evalNoKG},
		{"SL-CA", evalServiceLevel},
		{"ELCA", evalELCA},
		{"ConfigProfile", evalConfigProfile},
		{"OPP", evalOpportunistic},
		{"Static", evalStatic},
	}

	// Real-topology mode replaces the seeded synthetic mesh with the
	// service-dependency graphs found in topodir. In the artifact these are the
	// sampled Alibaba subgraphs. Only the topology is real. The scales are the
	// service counts of the files found.
	var realTopo map[int][][2]int
	if *scenario == "real-topology" {
		realTopo = loadTopologies(*topodir)
		scales = scales[:0]
		for n := range realTopo {
			scales = append(scales, n)
		}
		sort.Ints(scales)
	}

	type agg struct {
		comp    []float64
		valid   int
		unsafe  int
		blocked int
		managed int
	}

	// Synthetic and real-topology comparisons begin with no active assignment.
	// Appended outcome columns therefore partition valid/invalid/no-promotion,
	// while retained_active is explicitly zero. Existing columns are preserved.
	rows := [][]string{{"scale", "method", "correct_promotion_pct", "unsafe_promotion_pct", "unsafe_promotions", "blocked_safe", "managed_edges", "reps", "valid_promotions", "invalid_promotions", "no_promotion", "retained_active"}}
	for _, n := range scales {
		results := map[string]*agg{}
		for _, m := range methods {
			results[m.key] = &agg{}
		}
		for r := 0; r < *reps; r++ {
			var edges []genEdge
			if *scenario == "real-topology" {
				edges = meshFromTopology(realTopo[n], *seed+int64(r)*7919+int64(n))
			} else {
				edges = generateMesh(n, *seed+int64(r)*7919+int64(n))
			}
			for _, m := range methods {
				o := m.fn(edges, now)
				a := results[m.key]
				a.comp = append(a.comp, o.compliancePct())
				a.valid += o.compliant
				a.unsafe += o.unsafePromotion
				a.blocked += o.blockedSafe
				a.managed += o.managed
			}
		}
		for _, m := range methods {
			a := results[m.key]
			sort.Float64s(a.comp)
			med := a.comp[len(a.comp)/2]
			unsafePct := 100 * float64(a.unsafe) / float64(a.managed)
			rows = append(rows, []string{
				strconv.Itoa(n),
				m.key,
				strconv.FormatFloat(med, 'f', 2, 64),
				strconv.FormatFloat(unsafePct, 'f', 2, 64),
				strconv.Itoa(a.unsafe),
				strconv.Itoa(a.blocked),
				strconv.Itoa(a.managed),
				strconv.Itoa(*reps),
				strconv.Itoa(a.valid),
				strconv.Itoa(a.unsafe),
				strconv.Itoa(a.blocked),
				"0",
			})
		}
	}

	var w *csv.Writer
	if *out == "" {
		w = csv.NewWriter(os.Stdout)
	} else {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		w = csv.NewWriter(f)
	}
	if err := w.WriteAll(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	w.Flush()
}
